package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

type ProjectService struct {
	store ProjectStore
}

func NewProjectService(store ProjectStore) *ProjectService {
	return &ProjectService{store: store}
}

type CreateProjectInput struct {
	Name           string             `json:"name"`
	RawRequest     string             `json:"raw_request"`
	Mode           domain.ProjectMode `json:"mode"`
	OutputLanguage string             `json:"output_language"`
	Appetite       *domain.Appetite   `json:"appetite,omitempty"`
}

type UpdateProjectInput struct {
	ExpectedRevision int64               `json:"expected_revision"`
	Name             *string             `json:"name,omitempty"`
	Mode             *domain.ProjectMode `json:"mode,omitempty"`
	OutputLanguage   *string             `json:"output_language,omitempty"`
	Appetite         *domain.Appetite    `json:"appetite,omitempty"`
	ClearAppetite    bool                `json:"clear_appetite,omitempty"`
}

type StageGateCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type StageTransition struct {
	Next              domain.ProjectStage
	Actor             string
	Reopened          bool
	GateChecks        []StageGateCheck
	Unresolved        []string
	Reason            string
	ApprovalReference string
}

func (s *ProjectService) Create(ctx context.Context, input CreateProjectInput) (domain.Snapshot, error) {
	now := s.store.Now()
	mode := input.Mode
	if mode == "" {
		mode = domain.ModeGreenfield
	}
	language := strings.TrimSpace(input.OutputLanguage)
	if language == "" {
		language = "en"
	}
	project := domain.Project{
		ID:             domain.NewID("prj"),
		Name:           strings.TrimSpace(input.Name),
		RawRequest:     strings.TrimSpace(input.RawRequest),
		Mode:           mode,
		OutputLanguage: language,
		Stage:          domain.StageIntake,
		Status:         domain.ProjectActive,
		Appetite:       input.Appetite,
		Revision:       1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := domain.ValidateProject(project); err != nil {
		return domain.Snapshot{}, err
	}
	evidenceBody, err := json.Marshal(map[string]any{
		"evidence_type": "user_statement", "summary": project.RawRequest, "locator": "project.raw_request",
		"captured_at": now.Format(time.RFC3339Nano), "freshness": "current", "trust_notes": "Captured verbatim at project creation.",
	})
	if err != nil {
		return domain.Snapshot{}, err
	}
	evidence := domain.Entity{
		ID: domain.NewID("evidence"), ProjectID: project.ID, Kind: domain.KindEvidence, Title: "Initial project request", Body: evidenceBody,
		Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: 1, Freshness: domain.FreshnessCurrent,
		SourceRefs: []string{}, Tags: []string{"intake"}, CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
	return s.store.CreateProject(ctx, domain.Snapshot{SchemaVersion: "1", Project: project, Entities: []domain.Entity{evidence}, Relations: []domain.Relation{}}, "user")
}

func (s *ProjectService) List(ctx context.Context, includeArchived bool) ([]domain.Project, error) {
	return s.store.ListProjects(ctx, includeArchived)
}

func (s *ProjectService) Get(ctx context.Context, projectID string) (domain.Project, error) {
	snapshot, err := s.store.GetSnapshot(ctx, projectID)
	return snapshot.Project, err
}

func (s *ProjectService) Snapshot(ctx context.Context, projectID string) (domain.Snapshot, error) {
	return s.store.GetSnapshot(ctx, projectID)
}

func (s *ProjectService) Revisions(ctx context.Context, projectID string) ([]domain.ProjectRevision, error) {
	return s.store.ListRevisions(ctx, projectID)
}

func (s *ProjectService) Events(ctx context.Context, projectID string, afterSequence int64, limit int) ([]domain.Event, error) {
	if _, err := s.store.GetSnapshot(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListEvents(ctx, projectID, afterSequence, limit)
}

func (s *ProjectService) Update(ctx context.Context, projectID string, input UpdateProjectInput) (domain.Snapshot, error) {
	if input.ExpectedRevision < 1 {
		return domain.Snapshot{}, fmt.Errorf("%w: expected_revision must be positive", domain.ErrInvalid)
	}
	return s.store.Transact(ctx, projectID, input.ExpectedRevision, "user", func(snapshot *domain.Snapshot) ([]EventDraft, error) {
		if input.Name != nil {
			snapshot.Project.Name = strings.TrimSpace(*input.Name)
		}
		if input.Mode != nil {
			snapshot.Project.Mode = *input.Mode
		}
		if input.OutputLanguage != nil {
			snapshot.Project.OutputLanguage = strings.TrimSpace(*input.OutputLanguage)
		}
		if input.ClearAppetite {
			snapshot.Project.Appetite = nil
		} else if input.Appetite != nil {
			snapshot.Project.Appetite = input.Appetite
		}
		return []EventDraft{{Type: "project.updated", Payload: map[string]any{"expected_revision": input.ExpectedRevision}}}, nil
	})
}

func (s *ProjectService) Archive(ctx context.Context, projectID string, expectedRevision int64) (domain.Snapshot, error) {
	return s.setStatus(ctx, projectID, expectedRevision, domain.ProjectArchived, "project.archived")
}

func (s *ProjectService) Delete(ctx context.Context, projectID string, expectedRevision int64) error {
	if expectedRevision < 1 {
		return fmt.Errorf("%w: expected_revision must be positive", domain.ErrInvalid)
	}
	return s.store.DeleteProject(ctx, projectID, expectedRevision)
}

func (s *ProjectService) setStatus(ctx context.Context, projectID string, expectedRevision int64, status domain.ProjectStatus, eventType string) (domain.Snapshot, error) {
	if expectedRevision < 1 {
		return domain.Snapshot{}, fmt.Errorf("%w: expected_revision must be positive", domain.ErrInvalid)
	}
	return s.store.Transact(ctx, projectID, expectedRevision, "user", func(snapshot *domain.Snapshot) ([]EventDraft, error) {
		snapshot.Project.Status = status
		return []EventDraft{{Type: eventType, Payload: map[string]any{"status": status}}}, nil
	})
}

func (s *ProjectService) ChangeStage(ctx context.Context, projectID string, expectedRevision int64, transition StageTransition) (domain.Snapshot, error) {
	actor := strings.TrimSpace(transition.Actor)
	if actor == "" {
		actor = "workflow"
	}
	return s.store.Transact(ctx, projectID, expectedRevision, actor, func(snapshot *domain.Snapshot) ([]EventDraft, error) {
		previous := snapshot.Project.Stage
		if previous == transition.Next {
			return nil, fmt.Errorf("%w: project is already at stage %s", domain.ErrInvalid, transition.Next)
		}
		now := s.store.Now()
		if transition.Reopened {
			for index := range snapshot.Entities {
				entity := &snapshot.Entities[index]
				if entity.Origin == domain.OriginAgent && entity.Freshness == domain.FreshnessCurrent && (entity.Status == domain.EntityProposed || entity.Status == domain.EntityConfirmed) {
					entity.Freshness = domain.FreshnessPotentiallyStale
					entity.Revision++
					entity.UpdatedAt = now
				}
			}
		}
		snapshot.Project.Stage = transition.Next
		if transition.Next == domain.StageReady {
			snapshot.Project.Status = domain.ProjectReady
		}
		gateChecks := transition.GateChecks
		if gateChecks == nil {
			gateChecks = []StageGateCheck{}
		}
		unresolved := transition.Unresolved
		if unresolved == nil {
			unresolved = []string{}
		}
		payload := map[string]any{
			"previous": previous, "next": transition.Next, "gate_checks": gateChecks, "unresolved": unresolved,
			"model_revision": expectedRevision + 1, "actor": actor, "reopened": transition.Reopened, "reason": strings.TrimSpace(transition.Reason),
		}
		if transition.ApprovalReference != "" {
			payload["approval_reference"] = transition.ApprovalReference
		}
		return []EventDraft{{Type: "workflow.stage_changed", Payload: payload}}, nil
	})
}

func (s *ProjectService) ExportJSON(ctx context.Context, projectID string) ([]byte, error) {
	snapshot, err := s.store.GetSnapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode project export: %w", err)
	}
	return append(data, '\n'), nil
}
