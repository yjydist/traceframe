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
	ProjectRevision      int64              `json:"project_revision"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

type GateCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type State struct {
	ProjectID       string              `json:"project_id"`
	Stage           domain.ProjectStage `json:"stage"`
	ProjectRevision int64               `json:"project_revision"`
	GatePassed      bool                `json:"gate_passed"`
	Checks          []GateCheck         `json:"checks"`
	Blockers        []string            `json:"blockers"`
	Reason          string              `json:"reason"`
	Assessment      Assessment          `json:"assessment"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

type Store interface {
	SaveAssessment(ctx context.Context, assessment Assessment) error
	SaveState(ctx context.Context, state State) error
	RecordBlocked(ctx context.Context, projectID string, state State) error
}
