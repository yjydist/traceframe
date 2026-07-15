package application

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

type CommandEnvelope struct {
	ExpectedRevision int64     `json:"expected_revision"`
	Actor            string    `json:"actor,omitempty"`
	Commands         []Command `json:"commands"`
}

type Command struct {
	Type                   string         `json:"type"`
	Entity                 *EntityDraft   `json:"entity,omitempty"`
	EntityID               string         `json:"entity_id,omitempty"`
	ExpectedEntityRevision int64          `json:"expected_entity_revision,omitempty"`
	Changes                *EntityChanges `json:"changes,omitempty"`
	Relation               *RelationDraft `json:"relation,omitempty"`
	RelationID             string         `json:"relation_id,omitempty"`
}

type EntityDraft struct {
	ID         string              `json:"id,omitempty"`
	Kind       domain.EntityKind   `json:"kind"`
	Title      string              `json:"title"`
	Body       json.RawMessage     `json:"body"`
	Status     domain.EntityStatus `json:"status,omitempty"`
	Origin     domain.Origin       `json:"origin,omitempty"`
	Confidence *float64            `json:"confidence,omitempty"`
	Freshness  domain.Freshness    `json:"freshness,omitempty"`
	SourceRefs []string            `json:"source_refs,omitempty"`
	Tags       []string            `json:"tags,omitempty"`
}

type EntityChanges struct {
	Title      *string              `json:"title,omitempty"`
	Body       json.RawMessage      `json:"body,omitempty"`
	Status     *domain.EntityStatus `json:"status,omitempty"`
	Confidence *float64             `json:"confidence,omitempty"`
	Freshness  *domain.Freshness    `json:"freshness,omitempty"`
	SourceRefs *[]string            `json:"source_refs,omitempty"`
	Tags       *[]string            `json:"tags,omitempty"`
}

type RelationDraft struct {
	ID        string              `json:"id,omitempty"`
	FromID    string              `json:"from_id"`
	Type      domain.RelationType `json:"type"`
	ToID      string              `json:"to_id"`
	Rationale string              `json:"rationale"`
}

func (s *ProjectService) ApplyCommands(ctx context.Context, projectID string, envelope CommandEnvelope) (domain.Snapshot, error) {
	if envelope.ExpectedRevision < 1 {
		return domain.Snapshot{}, fmt.Errorf("%w: expected_revision must be positive", domain.ErrInvalid)
	}
	if len(envelope.Commands) == 0 {
		return domain.Snapshot{}, fmt.Errorf("%w: at least one command is required", domain.ErrInvalid)
	}
	if len(envelope.Commands) > 100 {
		return domain.Snapshot{}, fmt.Errorf("%w: command set exceeds the limit of 100", domain.ErrInvalid)
	}
	actor := strings.TrimSpace(envelope.Actor)
	if actor == "" {
		actor = "user"
	}

	return s.store.Transact(ctx, projectID, envelope.ExpectedRevision, actor, func(snapshot *domain.Snapshot) ([]EventDraft, error) {
		events := make([]EventDraft, 0, len(envelope.Commands))
		for index, command := range envelope.Commands {
			event, err := s.applyCommand(snapshot, actor, command)
			if err != nil {
				return nil, fmt.Errorf("command %d: %w", index, err)
			}
			events = append(events, event)
		}
		return events, nil
	})
}

func (s *ProjectService) applyCommand(snapshot *domain.Snapshot, actor string, command Command) (EventDraft, error) {
	switch command.Type {
	case "create_entity":
		return s.createEntity(snapshot, command.Entity)
	case "update_entity":
		return s.updateEntity(snapshot, command.EntityID, command.ExpectedEntityRevision, command.Changes)
	case "create_relation":
		return s.createRelation(snapshot, actor, command.Relation)
	case "delete_relation":
		return deleteRelation(snapshot, command.RelationID)
	default:
		return EventDraft{}, fmt.Errorf("%w: unsupported command type %q", domain.ErrInvalid, command.Type)
	}
}

func (s *ProjectService) createEntity(snapshot *domain.Snapshot, draft *EntityDraft) (EventDraft, error) {
	if draft == nil {
		return EventDraft{}, fmt.Errorf("%w: create_entity requires entity", domain.ErrInvalid)
	}
	id := strings.TrimSpace(draft.ID)
	if id == "" {
		id = domain.NewID(entityPrefix(draft.Kind))
	}
	if _, exists := findEntity(snapshot.Entities, id); exists {
		return EventDraft{}, fmt.Errorf("%w: entity %s already exists", ErrConflict, id)
	}
	status := draft.Status
	if status == "" {
		status = domain.EntityDraft
	}
	origin := draft.Origin
	if origin == "" {
		origin = domain.OriginUser
	}
	confidence := 1.0
	if draft.Confidence != nil {
		confidence = *draft.Confidence
	}
	freshness := draft.Freshness
	if freshness == "" {
		freshness = domain.FreshnessCurrent
	}
	body, err := domain.CanonicalJSON(draft.Body)
	if err != nil {
		return EventDraft{}, err
	}
	now := s.store.Now()
	entity := domain.Entity{
		ID: id, ProjectID: snapshot.Project.ID, Kind: draft.Kind, Title: strings.TrimSpace(draft.Title), Body: body,
		Status: status, Origin: origin, Confidence: confidence, Freshness: freshness,
		SourceRefs: append([]string{}, draft.SourceRefs...), Tags: append([]string{}, draft.Tags...),
		CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
	if err := domain.ValidateEntity(entity); err != nil {
		return EventDraft{}, err
	}
	snapshot.Entities = append(snapshot.Entities, entity)
	return EventDraft{Type: "project.entity_created", Payload: map[string]any{"entity_id": id, "kind": entity.Kind}}, nil
}

func (s *ProjectService) updateEntity(snapshot *domain.Snapshot, entityID string, expectedRevision int64, changes *EntityChanges) (EventDraft, error) {
	index, exists := findEntity(snapshot.Entities, entityID)
	if !exists {
		return EventDraft{}, fmt.Errorf("%w: entity %s", ErrNotFound, entityID)
	}
	if changes == nil {
		return EventDraft{}, fmt.Errorf("%w: update_entity requires changes", domain.ErrInvalid)
	}
	entity := snapshot.Entities[index]
	wasConfirmed := entity.Status == domain.EntityConfirmed
	if entity.Revision != expectedRevision {
		return EventDraft{}, fmt.Errorf("%w: expected entity revision %d, current revision is %d", ErrConflict, expectedRevision, entity.Revision)
	}
	if changes.Title != nil {
		entity.Title = strings.TrimSpace(*changes.Title)
	}
	if changes.Body != nil {
		body, err := domain.CanonicalJSON(changes.Body)
		if err != nil {
			return EventDraft{}, err
		}
		entity.Body = body
	}
	if changes.Status != nil {
		entity.Status = *changes.Status
	}
	if changes.Confidence != nil {
		entity.Confidence = *changes.Confidence
	}
	if changes.Freshness != nil {
		entity.Freshness = *changes.Freshness
	}
	if changes.SourceRefs != nil {
		entity.SourceRefs = append([]string{}, (*changes.SourceRefs)...)
	}
	if changes.Tags != nil {
		entity.Tags = append([]string{}, (*changes.Tags)...)
	}
	entity.Revision++
	entity.UpdatedAt = s.store.Now()
	if err := domain.ValidateEntity(entity); err != nil {
		return EventDraft{}, err
	}
	snapshot.Entities[index] = entity
	impacted := []string{}
	if wasConfirmed && materialEntityChange(changes) {
		impacted = markRelatedEntitiesPotentiallyStale(snapshot, entity.ID, s.store.Now())
	}
	return EventDraft{Type: "project.entity_updated", Payload: map[string]any{"entity_id": entity.ID, "entity_revision": entity.Revision, "impacted_entity_ids": impacted}}, nil
}

func materialEntityChange(changes *EntityChanges) bool {
	return changes.Title != nil || changes.Body != nil || changes.Status != nil || changes.SourceRefs != nil
}

func markRelatedEntitiesPotentiallyStale(snapshot *domain.Snapshot, sourceID string, now time.Time) []string {
	neighbors := make(map[string][]string)
	for _, relation := range snapshot.Relations {
		neighbors[relation.FromID] = append(neighbors[relation.FromID], relation.ToID)
		neighbors[relation.ToID] = append(neighbors[relation.ToID], relation.FromID)
	}
	seen := map[string]bool{sourceID: true}
	queue := append([]string{}, neighbors[sourceID]...)
	impacted := make([]string, 0)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		index, ok := findEntity(snapshot.Entities, id)
		if !ok {
			continue
		}
		entity := &snapshot.Entities[index]
		if (entity.Status == domain.EntityProposed || entity.Status == domain.EntityConfirmed) && entity.Freshness == domain.FreshnessCurrent {
			entity.Freshness = domain.FreshnessPotentiallyStale
			entity.Revision++
			entity.UpdatedAt = now
			impacted = append(impacted, entity.ID)
		}
		queue = append(queue, neighbors[id]...)
	}
	slices.Sort(impacted)
	return impacted
}

func (s *ProjectService) createRelation(snapshot *domain.Snapshot, actor string, draft *RelationDraft) (EventDraft, error) {
	if draft == nil {
		return EventDraft{}, fmt.Errorf("%w: create_relation requires relation", domain.ErrInvalid)
	}
	id := strings.TrimSpace(draft.ID)
	if id == "" {
		id = domain.NewID("rel")
	}
	for _, relation := range snapshot.Relations {
		if relation.ID == id || (relation.FromID == draft.FromID && relation.Type == draft.Type && relation.ToID == draft.ToID) {
			return EventDraft{}, fmt.Errorf("%w: relation already exists", ErrConflict)
		}
	}
	fromIndex, fromExists := findEntity(snapshot.Entities, draft.FromID)
	toIndex, toExists := findEntity(snapshot.Entities, draft.ToID)
	if !fromExists || !toExists {
		return EventDraft{}, fmt.Errorf("%w: relation endpoint", ErrNotFound)
	}
	relation := domain.Relation{
		ID: id, ProjectID: snapshot.Project.ID, FromID: draft.FromID, Type: draft.Type, ToID: draft.ToID,
		Rationale: strings.TrimSpace(draft.Rationale), CreatedBy: actor, CreatedAt: s.store.Now(),
	}
	if err := domain.ValidateRelation(relation, snapshot.Entities[fromIndex], snapshot.Entities[toIndex]); err != nil {
		return EventDraft{}, err
	}
	snapshot.Relations = append(snapshot.Relations, relation)
	return EventDraft{Type: "project.relation_created", Payload: map[string]any{"relation_id": id, "type": relation.Type}}, nil
}

func deleteRelation(snapshot *domain.Snapshot, relationID string) (EventDraft, error) {
	for index, relation := range snapshot.Relations {
		if relation.ID == relationID {
			snapshot.Relations = append(snapshot.Relations[:index], snapshot.Relations[index+1:]...)
			return EventDraft{Type: "project.relation_deleted", Payload: map[string]string{"relation_id": relationID}}, nil
		}
	}
	return EventDraft{}, fmt.Errorf("%w: relation %s", ErrNotFound, relationID)
}

func findEntity(entities []domain.Entity, entityID string) (int, bool) {
	for index := range entities {
		if entities[index].ID == entityID {
			return index, true
		}
	}
	return 0, false
}

func entityPrefix(kind domain.EntityKind) string {
	prefixes := map[domain.EntityKind]string{
		domain.KindGoal: "goal", domain.KindStakeholder: "stk", domain.KindContext: "ctx", domain.KindScopeItem: "scope",
		domain.KindConstraint: "con", domain.KindAssumption: "asm", domain.KindQuestion: "qst", domain.KindTerm: "term",
		domain.KindScenario: "scn", domain.KindRequirement: "req", domain.KindQualityScenario: "qsc", domain.KindRisk: "risk",
		domain.KindOption: "opt", domain.KindDecision: "dec", domain.KindSystemElement: "elem", domain.KindWorkSlice: "slice",
		domain.KindExperiment: "exp", domain.KindEvidence: "evidence", domain.KindVerification: "ver",
	}
	if prefix, ok := prefixes[kind]; ok {
		return prefix
	}
	return "ent"
}
