package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

func TestRepositoryPersistsAtomicVersionedProjectModel(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "traceframe.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repository := NewRepository(db)
	repository.now = func() time.Time { return createdAt }
	project := domain.Project{
		ID: "prj_test", Name: "Assignment planner", RawRequest: "Help students track assignments",
		Mode: domain.ModeGreenfield, OutputLanguage: "en", Stage: domain.StageIntake,
		Status: domain.ProjectActive, Revision: 1, CreatedAt: createdAt, UpdatedAt: createdAt,
	}

	snapshot, err := repository.CreateProject(ctx, project, "user")
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if snapshot.Project.Revision != 1 || snapshot.Project.Appetite != nil {
		t.Fatalf("created project = %#v", snapshot.Project)
	}

	entityCreatedAt := createdAt.Add(time.Minute)
	repository.now = func() time.Time { return entityCreatedAt }
	snapshot, err = repository.Transact(ctx, project.ID, 1, "user", func(snapshot *domain.Snapshot) ([]application.EventDraft, error) {
		snapshot.Entities = append(snapshot.Entities, domain.Entity{
			ID: "goal_test", ProjectID: project.ID, Kind: domain.KindGoal, Title: "Reduce missed deadlines",
			Body:   json.RawMessage(`{"priority":"must","success_signals":["Fewer late assignments"],"outcome":"Students submit work on time"}`),
			Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: 1, Freshness: domain.FreshnessCurrent,
			SourceRefs: []string{}, Tags: []string{"mvp"}, CreatedAt: entityCreatedAt, UpdatedAt: entityCreatedAt, Revision: 1,
		})
		return []application.EventDraft{{Type: "entity.created", Payload: map[string]string{"entity_id": "goal_test"}}}, nil
	})
	if err != nil {
		t.Fatalf("Transact(create entity) error = %v", err)
	}
	if snapshot.Project.Revision != 2 || len(snapshot.Entities) != 1 {
		t.Fatalf("snapshot after entity create = %#v", snapshot)
	}
	if string(snapshot.Entities[0].Body) != `{"outcome":"Students submit work on time","priority":"must","success_signals":["Fewer late assignments"]}` {
		t.Fatalf("entity body is not canonical: %s", snapshot.Entities[0].Body)
	}

	updatedAt := entityCreatedAt.Add(time.Minute)
	repository.now = func() time.Time { return updatedAt }
	snapshot, err = repository.Transact(ctx, project.ID, 2, "user", func(snapshot *domain.Snapshot) ([]application.EventDraft, error) {
		snapshot.Entities[0].Title = "Submit assignments on time"
		snapshot.Entities[0].Revision++
		snapshot.Entities[0].UpdatedAt = updatedAt
		return []application.EventDraft{{Type: "entity.updated", Payload: map[string]any{"entity_id": "goal_test", "revision": 2}}}, nil
	})
	if err != nil {
		t.Fatalf("Transact(update entity) error = %v", err)
	}
	if snapshot.Project.Revision != 3 || snapshot.Entities[0].Revision != 2 {
		t.Fatalf("snapshot revisions = project %d, entity %d", snapshot.Project.Revision, snapshot.Entities[0].Revision)
	}

	var entityVersions, projectRevisions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM entity_versions WHERE entity_id = 'goal_test'`).Scan(&entityVersions); err != nil {
		t.Fatalf("count entity versions: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM project_revisions WHERE project_id = 'prj_test'`).Scan(&projectRevisions); err != nil {
		t.Fatalf("count project revisions: %v", err)
	}
	if entityVersions != 2 || projectRevisions != 3 {
		t.Fatalf("version counts = entity %d, project %d", entityVersions, projectRevisions)
	}

	revisions, err := repository.ListRevisions(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListRevisions() error = %v", err)
	}
	if len(revisions) != 3 || revisions[0].Revision != 3 || revisions[0].Checksum == "" {
		t.Fatalf("revisions = %#v", revisions)
	}
	events, err := repository.ListEvents(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("event count = %d, want 5", len(events))
	}
}

func TestRepositoryRejectsConflictAndRollsBackInvalidMutation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "traceframe.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repository := NewRepository(db)
	repository.now = func() time.Time { return now }
	project := domain.Project{ID: "prj_atomic", Name: "Atomic", RawRequest: "Test atomic changes", Mode: domain.ModeSpike, OutputLanguage: "en", Stage: domain.StageIntake, Status: domain.ProjectActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := repository.CreateProject(ctx, project, "user"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	called := false
	_, err = repository.Transact(ctx, project.ID, 0, "user", func(snapshot *domain.Snapshot) ([]application.EventDraft, error) {
		called = true
		return nil, nil
	})
	if !errors.Is(err, application.ErrConflict) || called {
		t.Fatalf("conflict error = %v, mutation called = %v", err, called)
	}

	_, err = repository.Transact(ctx, project.ID, 1, "user", func(snapshot *domain.Snapshot) ([]application.EventDraft, error) {
		snapshot.Entities = append(snapshot.Entities, domain.Entity{
			ID: "goal_invalid", ProjectID: project.ID, Kind: domain.KindGoal, Title: "Invalid",
			Body: json.RawMessage(`{"outcome":"Missing fields"}`), Status: domain.EntityDraft, Origin: domain.OriginUser,
			Confidence: 1, Freshness: domain.FreshnessCurrent, Revision: 1, CreatedAt: now, UpdatedAt: now,
		})
		return []application.EventDraft{{Type: "should.not.persist", Payload: nil}}, nil
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid mutation error = %v, want ErrInvalid", err)
	}

	snapshot, err := repository.GetSnapshot(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if snapshot.Project.Revision != 1 || len(snapshot.Entities) != 0 {
		t.Fatalf("invalid transaction partially persisted: %#v", snapshot)
	}
	events, err := repository.ListEvents(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count after rollback = %d, want 1", len(events))
	}
}
