package workflow

import (
	"context"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

type Assessment struct {
	ProjectID            string             `json:"project_id"`
	Mode                 domain.ProjectMode `json:"mode"`
	SystemTypes          []string           `json:"system_types"`
	Criticality          string             `json:"criticality"`
	Novelty              int                `json:"novelty"`
	DomainUncertainty    int                `json:"domain_uncertainty"`
	TechnicalUncertainty int                `json:"technical_uncertainty"`
	ChangeScope          int                `json:"change_scope"`
	DataSensitivity      int                `json:"data_sensitivity"`
	OperationalExposure  int                `json:"operational_exposure"`
	ActiveConcerns       []string           `json:"active_concerns"`
	Corrected            bool               `json:"corrected"`
	ProjectRevision      int64              `json:"project_revision"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

type RoutedConcern struct {
	Name      string   `json:"name"`
	Mandatory bool     `json:"mandatory"`
	Triggers  []string `json:"triggers"`
}

type GateCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type State struct {
	ProjectID          string              `json:"project_id"`
	Stage              domain.ProjectStage `json:"stage"`
	ProjectRevision    int64               `json:"project_revision"`
	GatePassed         bool                `json:"gate_passed"`
	Checks             []GateCheck         `json:"checks"`
	Blockers           []string            `json:"blockers"`
	Concerns           []RoutedConcern     `json:"concerns"`
	RecommendedRoles   []domain.AgentRole  `json:"recommended_roles"`
	ApprovalReferences []string            `json:"approval_references"`
	Reason             string              `json:"reason"`
	Assessment         Assessment          `json:"assessment"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type Store interface {
	LoadAssessment(ctx context.Context, projectID string) (Assessment, bool, error)
	SaveAssessment(ctx context.Context, assessment Assessment) error
	SaveState(ctx context.Context, state State) error
	RecordBlocked(ctx context.Context, projectID string, state State) error
	RequestApproval(ctx context.Context, approval domain.Approval) (domain.Approval, bool, error)
	ListApprovals(ctx context.Context, projectID string) ([]domain.Approval, error)
	ResolveApproval(ctx context.Context, projectID, approvalID string, expectedProjectRevision int64, status domain.ApprovalStatus, actor, rationale string) (domain.Approval, error)
}

type AssessmentCorrection struct {
	ExpectedRevision int64    `json:"expected_revision"`
	Criticality      string   `json:"criticality"`
	ActiveConcerns   []string `json:"active_concerns"`
}

type ApprovalResolution struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Rationale        string `json:"rationale"`
}
