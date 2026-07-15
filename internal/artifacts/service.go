package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/review"
	"github.com/yjydist/traceframe/internal/workflow"
)

type WorkflowSource interface {
	Get(ctx context.Context, projectID string) (workflow.State, error)
}

type Service struct {
	projects *application.ProjectService
	workflow WorkflowSource
	store    Store
	now      func() time.Time
}

func NewService(projects *application.ProjectService, workflowSource WorkflowSource, store Store) *Service {
	return &Service{projects: projects, workflow: workflowSource, store: store, now: time.Now}
}

func (s *Service) Definitions(ctx context.Context, projectID string) ([]Definition, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	state, err := s.workflow.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return Route(snapshot, state.Concerns), nil
}

func (s *Service) Render(ctx context.Context, projectID string, request RenderRequest) (RenderResult, error) {
	snapshot, baseline, sourceChecksum, err := s.source(ctx, projectID, request.BaselineID)
	if err != nil {
		return RenderResult{}, err
	}
	if request.ExpectedRevision > 0 && request.ExpectedRevision != snapshot.Project.Revision {
		return RenderResult{}, fmt.Errorf("%w: expected project revision %d, source revision is %d", application.ErrConflict, request.ExpectedRevision, snapshot.Project.Revision)
	}
	concerns, err := s.routedConcerns(ctx, projectID, baseline)
	if err != nil {
		return RenderResult{}, err
	}
	definitions := Route(snapshot, concerns)
	renderers, err := normalizeRenderers(request.Renderers)
	if err != nil {
		return RenderResult{}, err
	}
	now := s.now().UTC()
	result := RenderResult{SourceRevision: snapshot.Project.Revision, BaselineID: request.BaselineID, Versions: []Version{}}
	for _, definition := range definitions {
		projection := projectionFor(definition, snapshot, baseline, sourceChecksum)
		if definition.Concern != "" && len(projection.Entities) == 0 {
			continue
		}
		for _, renderer := range renderers {
			if renderer == RendererMermaid && !supportsMermaid(definition.ViewType) {
				continue
			}
			content, contentType, err := Render(projection, renderer)
			if err != nil {
				return RenderResult{}, err
			}
			artifact, err := s.store.EnsureArtifact(ctx, Artifact{ID: domain.NewID("artifact"), ProjectID: projectID, ViewType: definition.ViewType, Title: definition.Title, RendererType: renderer, Mandatory: definition.Mandatory, Concern: definition.Concern, CreatedAt: now})
			if err != nil {
				return RenderResult{}, err
			}
			digest := sha256.Sum256([]byte(content))
			generationID := "deterministic:revision:" + fmt.Sprint(snapshot.Project.Revision)
			if baseline != nil {
				generationID = "deterministic:baseline:" + baseline.ID
			}
			version := Version{ID: domain.NewID("artifact_version"), ArtifactID: artifact.ID, ProjectID: projectID, RendererType: renderer, RendererVersion: rendererVersion, SourceRevision: snapshot.Project.Revision, IncludedEntityIDs: entityIDs(projection.Entities), ContentType: contentType, Content: content, Checksum: hex.EncodeToString(digest[:]), GenerationRunID: generationID, CreatedAt: now}
			if baseline != nil {
				version.SourceBaselineID = baseline.ID
			}
			version, _, err = s.store.SaveVersion(ctx, version)
			if err != nil {
				return RenderResult{}, err
			}
			result.Versions = append(result.Versions, version)
		}
	}
	slices.SortFunc(result.Versions, func(a, b Version) int {
		if a.ArtifactID < b.ArtifactID {
			return -1
		}
		if a.ArtifactID > b.ArtifactID {
			return 1
		}
		return 0
	})
	return result, nil
}

func (s *Service) List(ctx context.Context, projectID string) ([]Artifact, error) {
	if _, err := s.projects.Snapshot(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListArtifacts(ctx, projectID)
}

func (s *Service) Get(ctx context.Context, projectID, artifactID string) (Artifact, error) {
	return s.store.GetArtifact(ctx, projectID, artifactID)
}

func (s *Service) ExportMarkdown(ctx context.Context, projectID string) (string, error) {
	baseline, ok, err := s.store.LatestBaseline(ctx, projectID)
	if err != nil {
		return "", err
	}
	request := RenderRequest{Renderers: []RendererType{RendererMarkdown}}
	if ok {
		request.BaselineID = baseline.ID
	} else {
		snapshot, err := s.projects.Snapshot(ctx, projectID)
		if err != nil {
			return "", err
		}
		request.ExpectedRevision = snapshot.Project.Revision
	}
	result, err := s.Render(ctx, projectID, request)
	if err != nil {
		return "", err
	}
	for _, version := range result.Versions {
		artifact, err := s.store.GetArtifact(ctx, projectID, version.ArtifactID)
		if err == nil && artifact.ViewType == "implementation_packet" {
			return version.Content, nil
		}
	}
	return "", fmt.Errorf("implementation packet was not rendered")
}

func (s *Service) CurrentMandatoryViews(ctx context.Context, projectID string, sourceRevision int64) (bool, []string, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return false, nil, err
	}
	if snapshot.Project.Revision != sourceRevision {
		return false, nil, fmt.Errorf("%w: source revision %d is not current", application.ErrConflict, sourceRevision)
	}
	state, err := s.workflow.Get(ctx, projectID)
	if err != nil {
		return false, nil, err
	}
	current, err := s.store.CurrentViewTypes(ctx, projectID, sourceRevision)
	if err != nil {
		return false, nil, err
	}
	missing := make([]string, 0)
	for _, definition := range Route(snapshot, state.Concerns) {
		if definition.Mandatory && !slices.Contains(current, definition.ViewType) {
			missing = append(missing, definition.ViewType)
		}
	}
	slices.Sort(missing)
	return len(missing) == 0, missing, nil
}

func (s *Service) RoutedConcernNames(ctx context.Context, projectID string) ([]string, error) {
	state, err := s.workflow.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(state.Concerns))
	for index, concern := range state.Concerns {
		result[index] = concern.Name
	}
	slices.Sort(result)
	return result, nil
}

func (s *Service) routedConcerns(ctx context.Context, projectID string, baseline *review.Baseline) ([]workflow.RoutedConcern, error) {
	if baseline == nil {
		state, err := s.workflow.Get(ctx, projectID)
		if err != nil {
			return nil, err
		}
		return state.Concerns, nil
	}
	result := make([]workflow.RoutedConcern, len(baseline.RoutedConcerns))
	for index, name := range baseline.RoutedConcerns {
		mandatory := name == "security" || name == "privacy" || name == "migration" || name == "compatibility"
		result[index] = workflow.RoutedConcern{Name: name, Mandatory: mandatory, Triggers: []string{"baseline"}}
	}
	return result, nil
}

func (s *Service) source(ctx context.Context, projectID, baselineID string) (domain.Snapshot, *review.Baseline, string, error) {
	if strings.TrimSpace(baselineID) != "" {
		baseline, err := s.store.GetBaseline(ctx, projectID, baselineID)
		if err != nil {
			return domain.Snapshot{}, nil, "", err
		}
		var snapshot domain.Snapshot
		if err := json.Unmarshal(baseline.Snapshot, &snapshot); err != nil {
			return domain.Snapshot{}, nil, "", fmt.Errorf("decode baseline snapshot: %w", err)
		}
		return snapshot, &baseline, baseline.Checksum, nil
	}
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return domain.Snapshot{}, nil, "", err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return domain.Snapshot{}, nil, "", err
	}
	digest := sha256.Sum256(data)
	return snapshot, nil, hex.EncodeToString(digest[:]), nil
}

func normalizeRenderers(input []RendererType) ([]RendererType, error) {
	if len(input) == 0 {
		return []RendererType{RendererHTML, RendererMarkdown, RendererJSON, RendererMermaid}, nil
	}
	result := make([]RendererType, 0, len(input))
	for _, renderer := range input {
		if renderer != RendererHTML && renderer != RendererMarkdown && renderer != RendererJSON && renderer != RendererMermaid {
			return nil, fmt.Errorf("%w: unsupported renderer %q", domain.ErrInvalid, renderer)
		}
		if !slices.Contains(result, renderer) {
			result = append(result, renderer)
		}
	}
	return result, nil
}

func projectionFor(definition Definition, snapshot domain.Snapshot, baseline *review.Baseline, checksum string) Projection {
	entities := make([]domain.Entity, 0)
	ids := make(map[string]struct{})
	for _, entity := range snapshot.Entities {
		if entity.Freshness == domain.FreshnessStale || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded || !slices.Contains(definition.Kinds, entity.Kind) {
			continue
		}
		entities = append(entities, entity)
		ids[entity.ID] = struct{}{}
	}
	slices.SortFunc(entities, func(a, b domain.Entity) int {
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}
		return strings.Compare(a.ID, b.ID)
	})
	relations := make([]domain.Relation, 0)
	for _, relation := range snapshot.Relations {
		_, from := ids[relation.FromID]
		_, to := ids[relation.ToID]
		if from && to {
			relations = append(relations, relation)
		}
	}
	slices.SortFunc(relations, func(a, b domain.Relation) int { return strings.Compare(a.ID, b.ID) })
	return Projection{Definition: definition, Snapshot: snapshot, Entities: entities, Relations: relations, Baseline: baseline, Checksum: checksum}
}

func entityIDs(entities []domain.Entity) []string {
	result := make([]string, len(entities))
	for index, entity := range entities {
		result[index] = entity.ID
	}
	return result
}
