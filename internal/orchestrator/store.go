package orchestrator

import (
	"context"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/jobs"
	"github.com/yjydist/traceframe/internal/models"
)

type Store interface {
	CreateRun(ctx context.Context, run domain.AgentRun, job jobs.Job) (domain.AgentRun, bool, error)
	GetRun(ctx context.Context, projectID, runID string) (domain.AgentRun, error)
	ListRuns(ctx context.Context, projectID string, limit int) ([]domain.AgentRun, error)
	TransitionRun(ctx context.Context, projectID, runID string, next domain.RunState, errorCode, errorMessage string) (domain.AgentRun, error)
	RequestCancellation(ctx context.Context, projectID, runID string) (domain.AgentRun, error)
	CancellationRequested(ctx context.Context, runID string) (bool, error)
	RecordModelResult(ctx context.Context, projectID, runID string, response models.GenerateResponse, usage domain.RunUsage) error
	RecordProposalOutcome(ctx context.Context, projectID, runID, checksum, outcome string) error
}
