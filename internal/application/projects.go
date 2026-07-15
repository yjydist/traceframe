package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	return s.store.CreateProject(ctx, project, "user")
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
