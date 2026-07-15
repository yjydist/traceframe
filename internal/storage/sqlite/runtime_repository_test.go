package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/jobs"
)

func TestRuntimeRepositoryIdempotencyLeaseRecoveryAndFailure(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	projectRepository := NewRepository(db)
	now := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC)
	projectRepository.now = func() time.Time { return now }
	project := domain.Project{ID: "prj_runtime", Name: "Runtime", RawRequest: "Frame a vague request", Mode: domain.ModeGreenfield, OutputLanguage: "en", Stage: domain.StageIntake, Status: domain.ProjectActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := projectRepository.CreateProject(ctx, domain.Snapshot{SchemaVersion: "1", Project: project}, "user"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	runtimeRepository := NewRuntimeRepository(db)
	runtimeRepository.now = func() time.Time { return now }
	run, job := testRunAndJob(project.ID, "run_runtime", "idem_runtime", "checksum_one", now, 2)
	created, wasCreated, err := runtimeRepository.CreateRun(ctx, run, job)
	if err != nil || !wasCreated || created.ID != run.ID {
		t.Fatalf("CreateRun() = %#v, %v, %v", created, wasCreated, err)
	}
	existing, wasCreated, err := runtimeRepository.CreateRun(ctx, run, job)
	if err != nil || wasCreated || existing.ID != run.ID {
		t.Fatalf("idempotent CreateRun() = %#v, %v, %v", existing, wasCreated, err)
	}
	run.RequestChecksum = "different"
	if _, _, err := runtimeRepository.CreateRun(ctx, run, job); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("idempotency mismatch error = %v", err)
	}

	claimed, err := runtimeRepository.Claim(ctx, "worker_a", now, time.Minute)
	if err != nil || claimed == nil || claimed.Attempts != 1 || claimed.State != jobs.Running {
		t.Fatalf("first Claim() = %#v, %v", claimed, err)
	}
	if _, err := runtimeRepository.TransitionRun(ctx, project.ID, run.ID, domain.RunPreparingContext, "", ""); err != nil {
		t.Fatalf("TransitionRun() error = %v", err)
	}
	recoveredAt := now.Add(2 * time.Minute)
	count, err := runtimeRepository.RecoverExpired(ctx, recoveredAt)
	if err != nil || count != 1 {
		t.Fatalf("RecoverExpired() = %d, %v", count, err)
	}
	recoveredRun, err := runtimeRepository.GetRun(ctx, project.ID, run.ID)
	if err != nil || recoveredRun.State != domain.RunQueued {
		t.Fatalf("recovered run = %#v, %v", recoveredRun, err)
	}

	claimed, err = runtimeRepository.Claim(ctx, "worker_b", recoveredAt, time.Minute)
	if err != nil || claimed == nil || claimed.Attempts != 2 {
		t.Fatalf("second Claim() = %#v, %v", claimed, err)
	}
	count, err = runtimeRepository.RecoverExpired(ctx, recoveredAt.Add(2*time.Minute))
	if err != nil || count != 1 {
		t.Fatalf("final RecoverExpired() = %d, %v", count, err)
	}
	failedRun, err := runtimeRepository.GetRun(ctx, project.ID, run.ID)
	if err != nil || failedRun.State != domain.RunFailed || failedRun.ErrorCode != "lease_expired" {
		t.Fatalf("failed run = %#v, %v", failedRun, err)
	}
	var jobState jobs.State
	if err := db.QueryRow(`SELECT state FROM jobs WHERE id = ?`, job.ID).Scan(&jobState); err != nil || jobState != jobs.Failed {
		t.Fatalf("job state = %s, %v", jobState, err)
	}
}

func TestRuntimeRepositoryCancelsQueuedRunAtomically(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	now := time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC)
	projects := NewRepository(db)
	projects.now = func() time.Time { return now }
	project := domain.Project{ID: "prj_cancel", Name: "Cancel", RawRequest: "Cancel a run", Mode: domain.ModeSpike, OutputLanguage: "en", Stage: domain.StageIntake, Status: domain.ProjectActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
	if _, err := projects.CreateProject(ctx, domain.Snapshot{SchemaVersion: "1", Project: project}, "user"); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	runtimeRepository := NewRuntimeRepository(db)
	runtimeRepository.now = func() time.Time { return now }
	run, job := testRunAndJob(project.ID, "run_cancel", "idem_cancel", "checksum_cancel", now, 2)
	if _, _, err := runtimeRepository.CreateRun(ctx, run, job); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	cancelled, err := runtimeRepository.RequestCancellation(ctx, project.ID, run.ID)
	if err != nil || cancelled.State != domain.RunCancelled || cancelled.CancelRequestedAt == nil {
		t.Fatalf("RequestCancellation() = %#v, %v", cancelled, err)
	}
	var state jobs.State
	if err := db.QueryRow(`SELECT state FROM jobs WHERE run_id = ?`, run.ID).Scan(&state); err != nil || state != jobs.Cancelled {
		t.Fatalf("cancelled job state = %s, %v", state, err)
	}
	var stepCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_run_steps WHERE run_id = ?`, run.ID).Scan(&stepCount); err != nil || stepCount != 2 {
		t.Fatalf("cancelled run step count = %d, %v", stepCount, err)
	}
	claimed, err := runtimeRepository.Claim(ctx, "worker", now, time.Minute)
	if err != nil || claimed != nil {
		t.Fatalf("cancelled job was claimed: %#v, %v", claimed, err)
	}
}

func testRunAndJob(projectID, runID, idempotencyKey, checksum string, now time.Time, maxAttempts int) (domain.AgentRun, jobs.Job) {
	budget := domain.DefaultRunBudget()
	budget.MaxAttempts = maxAttempts
	run := domain.AgentRun{ID: runID, ProjectID: projectID, Role: domain.RoleDiscovery, State: domain.RunQueued, Task: "Identify framing gaps", BaseRevision: 1, Budget: budget, IdempotencyKey: idempotencyKey, RequestChecksum: checksum, PromptVersion: "discovery.v1", ResponseSchemaVersion: "proposal.v1", SelectedContextIDs: []string{}, AllowedTools: []string{}, CreatedAt: now, UpdatedAt: now}
	job := jobs.Job{ID: "job_" + runID, ProjectID: projectID, RunID: runID, Type: "agent_run", State: jobs.Queued, MaxAttempts: maxAttempts, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	return run, job
}
