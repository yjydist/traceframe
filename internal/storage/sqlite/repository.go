package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, now: time.Now}
}

func (r *Repository) Now() time.Time {
	return r.now().UTC()
}

func (r *Repository) CreateProject(ctx context.Context, snapshot domain.Snapshot, actor string) (domain.Snapshot, error) {
	if snapshot.SchemaVersion == "" {
		snapshot.SchemaVersion = "1"
	}
	if snapshot.Entities == nil {
		snapshot.Entities = []domain.Entity{}
	}
	if snapshot.Relations == nil {
		snapshot.Relations = []domain.Relation{}
	}
	if err := validateSnapshot(snapshot); err != nil {
		return domain.Snapshot{}, err
	}
	project := snapshot.Project
	snapshotBytes, checksum, err := encodeSnapshot(snapshot)
	if err != nil {
		return domain.Snapshot{}, err
	}
	appetite, err := marshalAppetite(project.Appetite)
	if err != nil {
		return domain.Snapshot{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("begin create project: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO projects (id, name, raw_request, mode, output_language, stage, status, appetite_json, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ID, project.Name, project.RawRequest, project.Mode, project.OutputLanguage, project.Stage, project.Status, appetite,
		project.Revision, formatTime(project.CreatedAt), formatTime(project.UpdatedAt))
	if err != nil {
		if isConstraintError(err) {
			return domain.Snapshot{}, fmt.Errorf("%w: project %s already exists", application.ErrConflict, project.ID)
		}
		return domain.Snapshot{}, fmt.Errorf("insert project: %w", err)
	}
	if err := insertRevision(ctx, tx, project.ID, project.Revision, checksum, snapshotBytes, actor, project.CreatedAt); err != nil {
		return domain.Snapshot{}, err
	}
	empty := domain.Snapshot{SchemaVersion: snapshot.SchemaVersion, Project: project, Entities: []domain.Entity{}, Relations: []domain.Relation{}}
	if err := persistEntities(ctx, tx, empty, snapshot); err != nil {
		return domain.Snapshot{}, err
	}
	if err := persistRelations(ctx, tx, empty, snapshot); err != nil {
		return domain.Snapshot{}, err
	}
	if err := insertEvent(ctx, tx, project.ID, "project.revision_created", map[string]any{"revision": project.Revision, "checksum": checksum}, project.CreatedAt); err != nil {
		return domain.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Snapshot{}, fmt.Errorf("commit create project: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) DeleteProject(ctx context.Context, projectID string, expectedRevision int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete project: %w", err)
	}
	defer tx.Rollback()

	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE id = ?`, projectID).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: project %s", application.ErrNotFound, projectID)
		}
		return fmt.Errorf("load project for deletion: %w", err)
	}
	if currentRevision != expectedRevision {
		return fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, expectedRevision, currentRevision)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: project %s", application.ErrNotFound, projectID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete project: %w", err)
	}
	return nil
}

func (r *Repository) ListProjects(ctx context.Context, includeArchived bool) ([]domain.Project, error) {
	query := `SELECT id, name, raw_request, mode, output_language, stage, status, appetite_json, revision, created_at, updated_at
		FROM projects WHERE status != 'deleted'`
	if !includeArchived {
		query += ` AND status != 'archived'`
	}
	query += ` ORDER BY updated_at DESC, id`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projects := make([]domain.Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func (r *Repository) GetSnapshot(ctx context.Context, projectID string) (domain.Snapshot, error) {
	return loadSnapshot(ctx, r.db, projectID)
}

func (r *Repository) ListRevisions(ctx context.Context, projectID string) ([]domain.ProjectRevision, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT project_id, revision, checksum, actor, created_at
		FROM project_revisions WHERE project_id = ? ORDER BY revision DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project revisions: %w", err)
	}
	defer rows.Close()

	revisions := make([]domain.ProjectRevision, 0)
	for rows.Next() {
		var revision domain.ProjectRevision
		var createdAt string
		if err := rows.Scan(&revision.ProjectID, &revision.Revision, &revision.Checksum, &revision.Actor, &createdAt); err != nil {
			return nil, fmt.Errorf("scan project revision: %w", err)
		}
		revision.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list project revisions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close project revisions: %w", err)
	}
	if len(revisions) == 0 {
		if _, err := r.GetSnapshot(ctx, projectID); err != nil {
			return nil, err
		}
	}
	return revisions, nil
}

func (r *Repository) Transact(ctx context.Context, projectID string, expectedRevision int64, actor string, mutate application.Mutation) (domain.Snapshot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("begin project transaction: %w", err)
	}
	defer tx.Rollback()

	before, err := loadSnapshot(ctx, tx, projectID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	if before.Project.Revision != expectedRevision {
		return domain.Snapshot{}, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, expectedRevision, before.Project.Revision)
	}
	after, err := cloneSnapshot(before)
	if err != nil {
		return domain.Snapshot{}, err
	}
	events, err := mutate(&after)
	if err != nil {
		return domain.Snapshot{}, err
	}

	now := r.Now()
	if after.Project.ID != before.Project.ID || !after.Project.CreatedAt.Equal(before.Project.CreatedAt) {
		return domain.Snapshot{}, fmt.Errorf("%w: project identity and creation time are immutable", domain.ErrInvalid)
	}
	after.Project.Revision = before.Project.Revision + 1
	after.Project.UpdatedAt = now
	if before.Project.Status == domain.ProjectReady && after.Project.Status == domain.ProjectReady {
		after.Project.Status = domain.ProjectActive
	}
	if err := validateSnapshot(after); err != nil {
		return domain.Snapshot{}, err
	}
	canonicalizeSnapshot(&after)
	snapshotBytes, checksum, err := encodeSnapshot(after)
	if err != nil {
		return domain.Snapshot{}, err
	}

	if err := persistProject(ctx, tx, after.Project); err != nil {
		return domain.Snapshot{}, err
	}
	if err := persistEntities(ctx, tx, before, after); err != nil {
		return domain.Snapshot{}, err
	}
	if err := persistRelations(ctx, tx, before, after); err != nil {
		return domain.Snapshot{}, err
	}
	if err := insertRevision(ctx, tx, projectID, after.Project.Revision, checksum, snapshotBytes, actor, now); err != nil {
		return domain.Snapshot{}, err
	}
	for _, event := range append(events, application.EventDraft{Type: "project.revision_created", Payload: map[string]any{"revision": after.Project.Revision, "checksum": checksum}}) {
		if err := insertEvent(ctx, tx, projectID, event.Type, event.Payload, now); err != nil {
			return domain.Snapshot{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Snapshot{}, fmt.Errorf("commit project transaction: %w", err)
	}
	return after, nil
}

func (r *Repository) ListEvents(ctx context.Context, projectID string, afterSequence int64, limit int) ([]domain.Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT sequence, project_id, type, payload_json, occurred_at
		FROM events WHERE project_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`, projectID, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.Event, 0)
	for rows.Next() {
		var event domain.Event
		var payload, occurredAt string
		if err := rows.Scan(&event.Sequence, &event.ProjectID, &event.Type, &payload, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		event.OccurredAt, err = parseTime(occurredAt)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events, nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadSnapshot(ctx context.Context, db queryer, projectID string) (domain.Snapshot, error) {
	project, err := scanProject(db.QueryRowContext(ctx, `
		SELECT id, name, raw_request, mode, output_language, stage, status, appetite_json, revision, created_at, updated_at
		FROM projects WHERE id = ? AND status != 'deleted'`, projectID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Snapshot{}, fmt.Errorf("%w: project %s", application.ErrNotFound, projectID)
		}
		return domain.Snapshot{}, err
	}

	entities, err := loadEntities(ctx, db, projectID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	relations, err := loadRelations(ctx, db, projectID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return domain.Snapshot{SchemaVersion: "1", Project: project, Entities: entities, Relations: relations}, nil
}

type scanner interface {
	Scan(...any) error
}

func scanProject(row scanner) (domain.Project, error) {
	var project domain.Project
	var appetite sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&project.ID, &project.Name, &project.RawRequest, &project.Mode, &project.OutputLanguage, &project.Stage, &project.Status, &appetite, &project.Revision, &createdAt, &updatedAt); err != nil {
		return domain.Project{}, err
	}
	if appetite.Valid {
		project.Appetite = &domain.Appetite{}
		if err := json.Unmarshal([]byte(appetite.String), project.Appetite); err != nil {
			return domain.Project{}, fmt.Errorf("decode project appetite: %w", err)
		}
	}
	var err error
	project.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	project.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func loadEntities(ctx context.Context, db queryer, projectID string) ([]domain.Entity, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, project_id, kind, title, body_json, status, origin, confidence, freshness, source_refs_json, tags_json, created_at, updated_at, revision
		FROM entities WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}
	defer rows.Close()
	entities := make([]domain.Entity, 0)
	for rows.Next() {
		var entity domain.Entity
		var body, sourceRefs, tags, createdAt, updatedAt string
		if err := rows.Scan(&entity.ID, &entity.ProjectID, &entity.Kind, &entity.Title, &body, &entity.Status, &entity.Origin, &entity.Confidence, &entity.Freshness, &sourceRefs, &tags, &createdAt, &updatedAt, &entity.Revision); err != nil {
			return nil, fmt.Errorf("scan entity: %w", err)
		}
		entity.Body = json.RawMessage(body)
		if err := json.Unmarshal([]byte(sourceRefs), &entity.SourceRefs); err != nil {
			return nil, fmt.Errorf("decode entity source refs: %w", err)
		}
		if err := json.Unmarshal([]byte(tags), &entity.Tags); err != nil {
			return nil, fmt.Errorf("decode entity tags: %w", err)
		}
		entity.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		entity.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, rows.Err()
}

func loadRelations(ctx context.Context, db queryer, projectID string) ([]domain.Relation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, project_id, from_id, type, to_id, rationale, created_by, created_at
		FROM relations WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("load relations: %w", err)
	}
	defer rows.Close()
	relations := make([]domain.Relation, 0)
	for rows.Next() {
		var relation domain.Relation
		var createdAt string
		if err := rows.Scan(&relation.ID, &relation.ProjectID, &relation.FromID, &relation.Type, &relation.ToID, &relation.Rationale, &relation.CreatedBy, &createdAt); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		relation.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		relations = append(relations, relation)
	}
	return relations, rows.Err()
}

func persistProject(ctx context.Context, tx *sql.Tx, project domain.Project) error {
	appetite, err := marshalAppetite(project.Appetite)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE projects SET name = ?, raw_request = ?, mode = ?, output_language = ?, stage = ?, status = ?, appetite_json = ?, revision = ?, updated_at = ?
		WHERE id = ?`, project.Name, project.RawRequest, project.Mode, project.OutputLanguage, project.Stage, project.Status, appetite, project.Revision, formatTime(project.UpdatedAt), project.ID)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: project %s", application.ErrNotFound, project.ID)
	}
	return nil
}

func persistEntities(ctx context.Context, tx *sql.Tx, before, after domain.Snapshot) error {
	previous := make(map[string]domain.Entity, len(before.Entities))
	for _, entity := range before.Entities {
		previous[entity.ID] = entity
	}
	current := make(map[string]struct{}, len(after.Entities))
	for _, entity := range after.Entities {
		current[entity.ID] = struct{}{}
		old, existed := previous[entity.ID]
		if existed && entitiesEqual(old, entity) {
			continue
		}
		if existed && entity.Revision != old.Revision+1 {
			return fmt.Errorf("%w: entity %s expected revision %d", application.ErrConflict, entity.ID, old.Revision+1)
		}
		if !existed && entity.Revision != 1 {
			return fmt.Errorf("%w: new entity %s must start at revision 1", domain.ErrInvalid, entity.ID)
		}
		if err := upsertEntity(ctx, tx, entity, after.Project.Revision); err != nil {
			return err
		}
	}
	for id := range previous {
		if _, exists := current[id]; !exists {
			return fmt.Errorf("%w: entities must be superseded rather than deleted", domain.ErrInvalid)
		}
	}
	return nil
}

func upsertEntity(ctx context.Context, tx *sql.Tx, entity domain.Entity, projectRevision int64) error {
	sourceRefs, _ := json.Marshal(nonNil(entity.SourceRefs))
	tags, _ := json.Marshal(nonNil(entity.Tags))
	result, err := tx.ExecContext(ctx, `
		INSERT INTO entities (id, project_id, kind, title, body_json, status, origin, confidence, freshness, source_refs_json, tags_json, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title = excluded.title, body_json = excluded.body_json, status = excluded.status,
			origin = excluded.origin, confidence = excluded.confidence, freshness = excluded.freshness,
			source_refs_json = excluded.source_refs_json, tags_json = excluded.tags_json, revision = excluded.revision, updated_at = excluded.updated_at
			WHERE entities.project_id = excluded.project_id`,
		entity.ID, entity.ProjectID, entity.Kind, entity.Title, string(entity.Body), entity.Status, entity.Origin, entity.Confidence,
		entity.Freshness, string(sourceRefs), string(tags), entity.Revision, formatTime(entity.CreatedAt), formatTime(entity.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert entity %s: %w", entity.ID, err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: entity id %s belongs to another project", application.ErrConflict, entity.ID)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO entity_versions (entity_id, project_id, entity_revision, project_revision, kind, title, body_json, status, origin, confidence, freshness, source_refs_json, tags_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entity.ID, entity.ProjectID, entity.Revision, projectRevision, entity.Kind, entity.Title, string(entity.Body), entity.Status, entity.Origin,
		entity.Confidence, entity.Freshness, string(sourceRefs), string(tags), formatTime(entity.CreatedAt), formatTime(entity.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert entity version %s/%d: %w", entity.ID, entity.Revision, err)
	}
	return nil
}

func persistRelations(ctx context.Context, tx *sql.Tx, before, after domain.Snapshot) error {
	previous := make(map[string]domain.Relation, len(before.Relations))
	for _, relation := range before.Relations {
		previous[relation.ID] = relation
	}
	current := make(map[string]struct{}, len(after.Relations))
	for _, relation := range after.Relations {
		current[relation.ID] = struct{}{}
		old, existed := previous[relation.ID]
		if existed && old != relation {
			return fmt.Errorf("%w: relations are immutable; create a replacement", domain.ErrInvalid)
		}
		if existed {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO relations (id, project_id, from_id, type, to_id, rationale, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			relation.ID, relation.ProjectID, relation.FromID, relation.Type, relation.ToID, relation.Rationale, relation.CreatedBy, formatTime(relation.CreatedAt))
		if err != nil {
			return fmt.Errorf("insert relation %s: %w", relation.ID, err)
		}
	}
	for id := range previous {
		if _, exists := current[id]; !exists {
			if _, err := tx.ExecContext(ctx, `DELETE FROM relations WHERE id = ? AND project_id = ?`, id, after.Project.ID); err != nil {
				return fmt.Errorf("delete relation %s: %w", id, err)
			}
		}
	}
	return nil
}

func validateSnapshot(snapshot domain.Snapshot) error {
	if err := domain.ValidateProject(snapshot.Project); err != nil {
		return err
	}
	entities := make(map[string]domain.Entity, len(snapshot.Entities))
	for i := range snapshot.Entities {
		entity := &snapshot.Entities[i]
		if entity.ProjectID != snapshot.Project.ID {
			return fmt.Errorf("%w: entity %s belongs to another project", domain.ErrInvalid, entity.ID)
		}
		canonical, err := domain.CanonicalJSON(entity.Body)
		if err != nil {
			return err
		}
		entity.Body = canonical
		if err := domain.ValidateEntity(*entity); err != nil {
			return err
		}
		if _, exists := entities[entity.ID]; exists {
			return fmt.Errorf("%w: duplicate entity id %s", domain.ErrInvalid, entity.ID)
		}
		entities[entity.ID] = *entity
	}
	for _, relation := range snapshot.Relations {
		from, fromExists := entities[relation.FromID]
		to, toExists := entities[relation.ToID]
		if !fromExists || !toExists {
			return fmt.Errorf("%w: relation %s references a missing endpoint", domain.ErrInvalid, relation.ID)
		}
		if err := domain.ValidateRelation(relation, from, to); err != nil {
			return err
		}
	}
	return nil
}

func canonicalizeSnapshot(snapshot *domain.Snapshot) {
	if snapshot.Entities == nil {
		snapshot.Entities = []domain.Entity{}
	}
	if snapshot.Relations == nil {
		snapshot.Relations = []domain.Relation{}
	}
	slices.SortFunc(snapshot.Entities, func(a, b domain.Entity) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	slices.SortFunc(snapshot.Relations, func(a, b domain.Relation) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
}

func encodeSnapshot(snapshot domain.Snapshot) ([]byte, string, error) {
	canonicalizeSnapshot(&snapshot)
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, "", fmt.Errorf("encode project snapshot: %w", err)
	}
	digest := sha256.Sum256(data)
	return data, hex.EncodeToString(digest[:]), nil
}

func cloneSnapshot(snapshot domain.Snapshot) (domain.Snapshot, error) {
	data, _, err := encodeSnapshot(snapshot)
	if err != nil {
		return domain.Snapshot{}, err
	}
	var clone domain.Snapshot
	if err := json.Unmarshal(data, &clone); err != nil {
		return domain.Snapshot{}, fmt.Errorf("clone project snapshot: %w", err)
	}
	return clone, nil
}

func insertRevision(ctx context.Context, tx *sql.Tx, projectID string, revision int64, checksum string, snapshot []byte, actor string, createdAt time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO project_revisions (project_id, revision, checksum, snapshot_json, actor, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, revision, checksum, string(snapshot), actor, formatTime(createdAt))
	if err != nil {
		return fmt.Errorf("insert project revision: %w", err)
	}
	return nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, projectID, eventType string, payload any, occurredAt time.Time) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode event payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events (project_id, type, payload_json, occurred_at) VALUES (?, ?, ?, ?)`,
		projectID, eventType, string(data), formatTime(occurredAt))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func entitiesEqual(a, b domain.Entity) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func nonNil(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

func marshalAppetite(value *domain.Appetite) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON field: %w", err)
	}
	return string(data), nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse persisted timestamp: %w", err)
	}
	return parsed, nil
}

func isConstraintError(err error) bool {
	return err != nil && (errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "constraint failed"))
}
