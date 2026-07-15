package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

type State string

const (
	Queued    State = "queued"
	Running   State = "running"
	Completed State = "completed"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

type Job struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	RunID          string     `json:"run_id"`
	Type           string     `json:"type"`
	State          State      `json:"state"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	AvailableAt    time.Time  `json:"available_at"`
	LeaseOwner     string     `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func Validate(job Job) error {
	if job.ID == "" || job.ProjectID == "" || job.RunID == "" || job.Type == "" {
		return fmt.Errorf("%w: job id, project_id, run_id, and type are required", domain.ErrInvalid)
	}
	if job.State != Queued && job.State != Running && job.State != Completed && job.State != Failed && job.State != Cancelled {
		return fmt.Errorf("%w: unsupported job state %q", domain.ErrInvalid, job.State)
	}
	if job.MaxAttempts < 1 || job.MaxAttempts > 5 || job.Attempts < 0 || job.Attempts > job.MaxAttempts {
		return fmt.Errorf("%w: invalid job attempt policy", domain.ErrInvalid)
	}
	return nil
}

type Store interface {
	Claim(ctx context.Context, workerID string, now time.Time, leaseDuration time.Duration) (*Job, error)
	Renew(ctx context.Context, jobID, workerID string, now time.Time, leaseDuration time.Duration) error
	Complete(ctx context.Context, jobID, workerID string, now time.Time) error
	Fail(ctx context.Context, jobID, workerID string, now, retryAt time.Time, cause error) (State, error)
	FailTerminal(ctx context.Context, jobID, workerID string, now time.Time, cause error) error
	CancelRun(ctx context.Context, runID string, now time.Time) error
	RecoverExpired(ctx context.Context, now time.Time) (int64, error)
}
