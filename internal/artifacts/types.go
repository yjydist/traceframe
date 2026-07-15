package artifacts

import (
	"context"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/review"
)

type RendererType string

const (
	RendererHTML     RendererType = "html"
	RendererMarkdown RendererType = "markdown"
	RendererJSON     RendererType = "json"
	RendererMermaid  RendererType = "mermaid"
	rendererVersion               = "1"
)

type Definition struct {
	ViewType  string              `json:"view_type"`
	Title     string              `json:"title"`
	Mandatory bool                `json:"mandatory"`
	Concern   string              `json:"concern,omitempty"`
	Kinds     []domain.EntityKind `json:"-"`
}

type Artifact struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	ViewType     string       `json:"view_type"`
	Title        string       `json:"title"`
	RendererType RendererType `json:"renderer_type"`
	Mandatory    bool         `json:"mandatory"`
	Concern      string       `json:"concern,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	Latest       *Version     `json:"latest,omitempty"`
}

type Version struct {
	ID                string       `json:"id"`
	ArtifactID        string       `json:"artifact_id"`
	ProjectID         string       `json:"project_id"`
	RendererType      RendererType `json:"renderer_type"`
	RendererVersion   string       `json:"renderer_version"`
	SourceRevision    int64        `json:"source_revision"`
	SourceBaselineID  string       `json:"source_baseline_id,omitempty"`
	IncludedEntityIDs []string     `json:"included_entity_ids"`
	ContentType       string       `json:"content_type"`
	Content           string       `json:"content"`
	Checksum          string       `json:"checksum"`
	GenerationRunID   string       `json:"generation_run_id"`
	Stale             bool         `json:"stale"`
	CreatedAt         time.Time    `json:"created_at"`
}

type RenderRequest struct {
	ExpectedRevision int64          `json:"expected_revision,omitempty"`
	BaselineID       string         `json:"baseline_id,omitempty"`
	Renderers        []RendererType `json:"renderers,omitempty"`
}

type RenderResult struct {
	SourceRevision int64     `json:"source_revision"`
	BaselineID     string    `json:"baseline_id,omitempty"`
	Versions       []Version `json:"versions"`
}

type Projection struct {
	Definition Definition
	Snapshot   domain.Snapshot
	Entities   []domain.Entity
	Relations  []domain.Relation
	Baseline   *review.Baseline
	Checksum   string
}

type Store interface {
	EnsureArtifact(ctx context.Context, artifact Artifact) (Artifact, error)
	SaveVersion(ctx context.Context, version Version) (Version, bool, error)
	ListArtifacts(ctx context.Context, projectID string) ([]Artifact, error)
	GetArtifact(ctx context.Context, projectID, artifactID string) (Artifact, error)
	GetBaseline(ctx context.Context, projectID, baselineID string) (review.Baseline, error)
	LatestBaseline(ctx context.Context, projectID string) (review.Baseline, bool, error)
	CurrentViewTypes(ctx context.Context, projectID string, sourceRevision int64) ([]string, error)
}
