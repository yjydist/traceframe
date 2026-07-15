package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type AgentRole string

const (
	RoleDiscovery    AgentRole = "discovery"
	RoleRequirements AgentRole = "requirements"
	RoleArchitecture AgentRole = "architecture"
	RoleQualityRisk  AgentRole = "quality_risk"
	RoleDelivery     AgentRole = "delivery"
	RoleCritic       AgentRole = "critic"
)

type RunState string

const (
	RunQueued           RunState = "queued"
	RunPreparingContext RunState = "preparing_context"
	RunWaitingForModel  RunState = "waiting_for_model"
	RunUsingTool        RunState = "using_tool"
	RunValidating       RunState = "validating"
	RunAwaitingApproval RunState = "awaiting_approval"
	RunCompleted        RunState = "completed"
	RunFailed           RunState = "failed"
	RunCancelled        RunState = "cancelled"
)

type RunBudget struct {
	MaxModelTurns    int           `json:"max_model_turns"`
	MaxToolCalls     int           `json:"max_tool_calls"`
	MaxInputTokens   int           `json:"max_input_tokens"`
	MaxOutputTokens  int           `json:"max_output_tokens"`
	WallClockTimeout time.Duration `json:"wall_clock_timeout"`
	MaxAttempts      int           `json:"max_attempts"`
}

func DefaultRunBudget() RunBudget {
	return RunBudget{MaxModelTurns: 2, MaxToolCalls: 4, MaxInputTokens: 16_000, MaxOutputTokens: 4_000, WallClockTimeout: 2 * time.Minute, MaxAttempts: 2}
}

func (b RunBudget) Validate() error {
	if b.MaxModelTurns < 1 || b.MaxModelTurns > 20 {
		return fmt.Errorf("%w: max_model_turns must be between 1 and 20", ErrInvalid)
	}
	if b.MaxToolCalls < 0 || b.MaxToolCalls > 100 {
		return fmt.Errorf("%w: max_tool_calls must be between 0 and 100", ErrInvalid)
	}
	if b.MaxInputTokens < 1 || b.MaxOutputTokens < 1 {
		return fmt.Errorf("%w: token budgets must be positive", ErrInvalid)
	}
	if b.WallClockTimeout < time.Second || b.WallClockTimeout > 30*time.Minute {
		return fmt.Errorf("%w: wall_clock_timeout must be between one second and 30 minutes", ErrInvalid)
	}
	if b.MaxAttempts < 1 || b.MaxAttempts > 5 {
		return fmt.Errorf("%w: max_attempts must be between 1 and 5", ErrInvalid)
	}
	return nil
}

type RunUsage struct {
	ModelTurns   int `json:"model_turns"`
	ToolCalls    int `json:"tool_calls"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type AgentRun struct {
	ID                    string     `json:"id"`
	ProjectID             string     `json:"project_id"`
	Role                  AgentRole  `json:"role"`
	State                 RunState   `json:"state"`
	Task                  string     `json:"task"`
	BaseRevision          int64      `json:"base_revision"`
	Budget                RunBudget  `json:"budget"`
	Usage                 RunUsage   `json:"usage"`
	IdempotencyKey        string     `json:"idempotency_key"`
	RequestChecksum       string     `json:"request_checksum"`
	PromptVersion         string     `json:"prompt_version"`
	ResponseSchemaVersion string     `json:"response_schema_version"`
	ModelIdentifier       string     `json:"model_identifier,omitempty"`
	ProviderRequestID     string     `json:"provider_request_id,omitempty"`
	SelectedContextIDs    []string   `json:"selected_context_ids"`
	AllowedTools          []string   `json:"allowed_tools"`
	ProposalChecksum      string     `json:"proposal_checksum,omitempty"`
	ApplicationOutcome    string     `json:"application_outcome,omitempty"`
	ErrorCode             string     `json:"error_code,omitempty"`
	ErrorMessage          string     `json:"error_message,omitempty"`
	CancelRequestedAt     *time.Time `json:"cancel_requested_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func ValidateAgentRun(run AgentRun) error {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.ProjectID) == "" || strings.TrimSpace(run.Task) == "" || strings.TrimSpace(run.IdempotencyKey) == "" || strings.TrimSpace(run.RequestChecksum) == "" {
		return fmt.Errorf("%w: run id, project_id, bounded task, idempotency_key, and request_checksum are required", ErrInvalid)
	}
	if strings.TrimSpace(run.PromptVersion) == "" || strings.TrimSpace(run.ResponseSchemaVersion) == "" {
		return fmt.Errorf("%w: prompt and response-schema versions are required", ErrInvalid)
	}
	if run.Role != RoleDiscovery && run.Role != RoleRequirements && run.Role != RoleArchitecture && run.Role != RoleQualityRisk && run.Role != RoleDelivery && run.Role != RoleCritic {
		return fmt.Errorf("%w: unsupported agent role %q", ErrInvalid, run.Role)
	}
	if !validRunState(run.State) {
		return fmt.Errorf("%w: unsupported run state %q", ErrInvalid, run.State)
	}
	if run.BaseRevision < 1 {
		return fmt.Errorf("%w: base revision must be positive", ErrInvalid)
	}
	if err := run.Budget.Validate(); err != nil {
		return err
	}
	return nil
}

func CanTransitionRun(from, to RunState) bool {
	if from == to {
		return false
	}
	if to == RunCancelled {
		return !RunTerminal(from)
	}
	switch from {
	case RunQueued:
		return to == RunPreparingContext || to == RunFailed
	case RunPreparingContext:
		return to == RunWaitingForModel || to == RunFailed
	case RunWaitingForModel:
		return to == RunUsingTool || to == RunValidating || to == RunFailed
	case RunUsingTool:
		return to == RunWaitingForModel || to == RunValidating || to == RunFailed
	case RunValidating:
		return to == RunAwaitingApproval || to == RunCompleted || to == RunFailed
	case RunAwaitingApproval:
		return to == RunCompleted || to == RunFailed
	default:
		return false
	}
}

func RunTerminal(state RunState) bool {
	return state == RunCompleted || state == RunFailed || state == RunCancelled
}

func ValidateRunTransition(from, to RunState) error {
	if !validRunState(from) || !validRunState(to) {
		return fmt.Errorf("%w: unknown run state transition %s -> %s", ErrInvalid, from, to)
	}
	if !CanTransitionRun(from, to) {
		return fmt.Errorf("%w: run transition %s -> %s is not allowed", ErrInvalid, from, to)
	}
	return nil
}

func validRunState(state RunState) bool {
	switch state {
	case RunQueued, RunPreparingContext, RunWaitingForModel, RunUsingTool, RunValidating, RunAwaitingApproval, RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

var ErrRunBudgetExceeded = errors.New("run budget exceeded")

func (b RunBudget) Check(usage RunUsage) error {
	if usage.ModelTurns > b.MaxModelTurns || usage.ToolCalls > b.MaxToolCalls || usage.InputTokens > b.MaxInputTokens || usage.OutputTokens > b.MaxOutputTokens {
		return ErrRunBudgetExceeded
	}
	return nil
}
