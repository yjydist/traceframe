package review

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityBlocking Severity = "blocking"
)

type FindingStatus string

const (
	FindingOpen         FindingStatus = "open"
	FindingResolved     FindingStatus = "resolved"
	FindingDismissed    FindingStatus = "dismissed"
	FindingRiskAccepted FindingStatus = "risk_accepted"
)

type FindingDraft struct {
	ID                    string   `json:"id"`
	Severity              Severity `json:"severity"`
	Category              string   `json:"category"`
	AffectedEntityIDs     []string `json:"affected_entity_ids"`
	Claim                 string   `json:"claim"`
	Evidence              string   `json:"evidence"`
	RecommendedResolution string   `json:"recommended_resolution"`
}

type Finding struct {
	ID                    string        `json:"id"`
	ProjectID             string        `json:"project_id"`
	RunID                 string        `json:"run_id"`
	ProjectRevision       int64         `json:"project_revision"`
	Severity              Severity      `json:"severity"`
	Category              string        `json:"category"`
	AffectedEntityIDs     []string      `json:"affected_entity_ids"`
	Claim                 string        `json:"claim"`
	Evidence              string        `json:"evidence"`
	RecommendedResolution string        `json:"recommended_resolution"`
	Status                FindingStatus `json:"status"`
	ResolutionRationale   string        `json:"resolution_rationale,omitempty"`
	CounterEvidenceRefs   []string      `json:"counter_evidence_refs"`
	ResolvedBy            string        `json:"resolved_by,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	ResolvedAt            *time.Time    `json:"resolved_at,omitempty"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

type Resolution struct {
	ExpectedRevision    int64         `json:"expected_revision"`
	Status              FindingStatus `json:"status"`
	Rationale           string        `json:"rationale"`
	CounterEvidenceRefs []string      `json:"counter_evidence_refs"`
}

type ReadinessCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type Readiness struct {
	ProjectID       string           `json:"project_id"`
	ProjectRevision int64            `json:"project_revision"`
	Ready           bool             `json:"ready"`
	Checks          []ReadinessCheck `json:"checks"`
	Blockers        []string         `json:"blockers"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type Baseline struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	ProjectRevision   int64           `json:"project_revision"`
	Checksum          string          `json:"checksum"`
	Snapshot          json.RawMessage `json:"snapshot,omitempty"`
	RoutedConcerns    []string        `json:"routed_concerns"`
	ApprovalActor     string          `json:"approval_actor"`
	ApprovalRationale string          `json:"approval_rationale"`
	ApprovedAt        time.Time       `json:"approved_at"`
	CreatedAt         time.Time       `json:"created_at"`
}

type BaselineRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Approve          bool   `json:"approve"`
	Rationale        string `json:"rationale"`
}

type Store interface {
	CreateFindings(ctx context.Context, projectID, runID string, projectRevision int64, findings []Finding) error
	ListFindings(ctx context.Context, projectID string) ([]Finding, error)
	ResolveFinding(ctx context.Context, projectID, findingID string, resolution Resolution, actor string) (Finding, error)
	ListApprovals(ctx context.Context, projectID string) ([]domain.Approval, error)
	CreateBaseline(ctx context.Context, baseline Baseline, expectedRevision int64) (Baseline, bool, error)
	ListBaselines(ctx context.Context, projectID string) ([]Baseline, error)
}
