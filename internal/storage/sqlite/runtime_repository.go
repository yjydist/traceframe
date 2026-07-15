package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/jobs"
	"github.com/yjydist/traceframe/internal/models"
)

type RuntimeRepository struct {
	db  *sql.DB
	now func() time.Time
}

func NewRuntimeRepository(db *sql.DB) *RuntimeRepository {
	return &RuntimeRepository{db: db, now: time.Now}
}

func (r *RuntimeRepository) CreateRun(ctx context.Context, run domain.AgentRun, job jobs.Job) (domain.AgentRun, bool, error) {
	if err := domain.ValidateAgentRun(run); err != nil {
		return domain.AgentRun{}, false, err
	}
	if err := jobs.Validate(job); err != nil {
		return domain.AgentRun{}, false, err
	}
	if run.ProjectID != job.ProjectID || run.ID != job.RunID || run.State != domain.RunQueued || job.State != jobs.Queued {
		return domain.AgentRun{}, false, fmt.Errorf("%w: run and job identity or initial state mismatch", domain.ErrInvalid)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AgentRun{}, false, fmt.Errorf("begin create run: %w", err)
	}
	defer tx.Rollback()

	existing, err := scanRun(tx.QueryRowContext(ctx, runSelect+` WHERE project_id = ? AND idempotency_key = ?`, run.ProjectID, run.IdempotencyKey))
	if err == nil {
		if existing.RequestChecksum != run.RequestChecksum {
			return domain.AgentRun{}, false, fmt.Errorf("%w: idempotency key was reused with a different request", application.ErrConflict)
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.AgentRun{}, false, err
	}

	budget, _ := json.Marshal(run.Budget)
	usage, _ := json.Marshal(run.Usage)
	contextIDs, _ := json.Marshal(nonNil(run.SelectedContextIDs))
	allowedTools, _ := json.Marshal(nonNil(run.AllowedTools))
	_, err = tx.ExecContext(ctx, `
		INSERT INTO agent_runs (id, project_id, role, state, task, base_revision, budget_json, usage_json, idempotency_key, request_checksum,
			prompt_version, response_schema_version, model_identifier, provider_request_id, selected_context_ids_json, allowed_tools_json,
			proposal_checksum, application_outcome, error_code, error_message, cancel_requested_at, created_at, started_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ProjectID, run.Role, run.State, run.Task, run.BaseRevision, string(budget), string(usage), run.IdempotencyKey, run.RequestChecksum,
		run.PromptVersion, run.ResponseSchemaVersion, run.ModelIdentifier, run.ProviderRequestID, string(contextIDs), string(allowedTools),
		run.ProposalChecksum, run.ApplicationOutcome, run.ErrorCode, run.ErrorMessage, nullableTime(run.CancelRequestedAt), formatTime(run.CreatedAt),
		nullableTime(run.StartedAt), nullableTime(run.CompletedAt), formatTime(run.UpdatedAt))
	if err != nil {
		return domain.AgentRun{}, false, fmt.Errorf("insert agent run: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO jobs (id, project_id, run_id, type, state, attempts, max_attempts, available_at, lease_owner, lease_expires_at, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.ProjectID, job.RunID, job.Type, job.State, job.Attempts, job.MaxAttempts,
		formatTime(job.AvailableAt), job.LeaseOwner, nullableTime(job.LeaseExpiresAt), job.LastError, formatTime(job.CreatedAt), formatTime(job.UpdatedAt))
	if err != nil {
		return domain.AgentRun{}, false, fmt.Errorf("insert job: %w", err)
	}
	if err := insertEvent(ctx, tx, run.ProjectID, "run.queued", map[string]any{"run_id": run.ID, "role": run.Role, "base_revision": run.BaseRevision}, run.CreatedAt); err != nil {
		return domain.AgentRun{}, false, err
	}
	if err := insertRunStep(ctx, tx, run.ID, 1, run.State, "Run persisted and queued.", run.CreatedAt); err != nil {
		return domain.AgentRun{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, false, fmt.Errorf("commit create run: %w", err)
	}
	return run, true, nil
}

func (r *RuntimeRepository) GetRun(ctx context.Context, projectID, runID string) (domain.AgentRun, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, runSelect+` WHERE project_id = ? AND id = ?`, projectID, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AgentRun{}, fmt.Errorf("%w: run %s", application.ErrNotFound, runID)
	}
	return run, err
}

func (r *RuntimeRepository) ListRuns(ctx context.Context, projectID string, limit int) ([]domain.AgentRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, runSelect+` WHERE project_id = ? ORDER BY created_at DESC, id LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	runs := make([]domain.AgentRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *RuntimeRepository) TransitionRun(ctx context.Context, projectID, runID string, next domain.RunState, errorCode, errorMessage string) (domain.AgentRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("begin run transition: %w", err)
	}
	defer tx.Rollback()
	run, err := scanRun(tx.QueryRowContext(ctx, runSelect+` WHERE project_id = ? AND id = ?`, projectID, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AgentRun{}, fmt.Errorf("%w: run %s", application.ErrNotFound, runID)
	}
	if err != nil {
		return domain.AgentRun{}, err
	}
	if err := domain.ValidateRunTransition(run.State, next); err != nil {
		return domain.AgentRun{}, err
	}
	now := r.now().UTC()
	previous := run.State
	run.State, run.UpdatedAt = next, now
	run.ErrorCode, run.ErrorMessage = errorCode, errorMessage
	if run.StartedAt == nil && next != domain.RunQueued {
		run.StartedAt = &now
	}
	if domain.RunTerminal(next) {
		run.CompletedAt = &now
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state = ?, error_code = ?, error_message = ?, started_at = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		run.State, run.ErrorCode, run.ErrorMessage, nullableTime(run.StartedAt), nullableTime(run.CompletedAt), formatTime(run.UpdatedAt), run.ID)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("update run state: %w", err)
	}
	if err := insertEvent(ctx, tx, projectID, "run.state_changed", map[string]any{"run_id": runID, "previous": previous, "state": next, "error_code": errorCode}, now); err != nil {
		return domain.AgentRun{}, err
	}
	var nextSequence int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM agent_run_steps WHERE run_id = ?`, runID).Scan(&nextSequence); err != nil {
		return domain.AgentRun{}, err
	}
	summary := "Run entered " + string(next) + "."
	if errorMessage != "" {
		summary = errorMessage
	}
	if err := insertRunStep(ctx, tx, runID, nextSequence, next, summary, now); err != nil {
		return domain.AgentRun{}, err
	}
	if domain.RunTerminal(next) {
		if err := insertEvent(ctx, tx, projectID, "run."+string(next), map[string]any{"run_id": runID, "error_code": errorCode}, now); err != nil {
			return domain.AgentRun{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, fmt.Errorf("commit run transition: %w", err)
	}
	return run, nil
}

func (r *RuntimeRepository) RequestCancellation(ctx context.Context, projectID, runID string) (domain.AgentRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AgentRun{}, err
	}
	defer tx.Rollback()
	run, err := scanRun(tx.QueryRowContext(ctx, runSelect+` WHERE project_id = ? AND id = ?`, projectID, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AgentRun{}, fmt.Errorf("%w: run %s", application.ErrNotFound, runID)
	}
	if err != nil || domain.RunTerminal(run.State) || run.CancelRequestedAt != nil {
		return run, err
	}
	now := r.now().UTC()
	run.CancelRequestedAt = &now
	if run.State == domain.RunQueued {
		run.State, run.CompletedAt = domain.RunCancelled, &now
		if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state = 'cancelled', lease_owner = '', lease_expires_at = NULL, updated_at = ? WHERE run_id = ? AND state = 'queued'`, formatTime(now), runID); err != nil {
			return domain.AgentRun{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state = ?, cancel_requested_at = ?, completed_at = ?, updated_at = ? WHERE id = ?`, run.State, formatTime(now), nullableTime(run.CompletedAt), formatTime(now), runID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	eventType := "run.cancel_requested"
	if run.State == domain.RunCancelled {
		eventType = "run.cancelled"
	}
	if err := insertEvent(ctx, tx, projectID, eventType, map[string]string{"run_id": runID}, now); err != nil {
		return domain.AgentRun{}, err
	}
	if run.State == domain.RunCancelled {
		var nextSequence int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM agent_run_steps WHERE run_id = ?`, runID).Scan(&nextSequence); err != nil {
			return domain.AgentRun{}, err
		}
		if err := insertRunStep(ctx, tx, runID, nextSequence, domain.RunCancelled, "Run cancelled before execution.", now); err != nil {
			return domain.AgentRun{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, err
	}
	return run, nil
}

func (r *RuntimeRepository) CancellationRequested(ctx context.Context, runID string) (bool, error) {
	var requested bool
	if err := r.db.QueryRowContext(ctx, `SELECT cancel_requested_at IS NOT NULL FROM agent_runs WHERE id = ?`, runID).Scan(&requested); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("%w: run %s", application.ErrNotFound, runID)
		}
		return false, err
	}
	return requested, nil
}

func (r *RuntimeRepository) RecordModelResult(ctx context.Context, projectID, runID string, response models.GenerateResponse, usage domain.RunUsage) error {
	usageJSON, _ := json.Marshal(usage)
	result, err := r.db.ExecContext(ctx, `UPDATE agent_runs SET model_identifier = ?, provider_request_id = ?, usage_json = ?, updated_at = ? WHERE project_id = ? AND id = ?`,
		response.ModelIdentifier, response.ProviderRequestID, string(usageJSON), formatTime(r.now().UTC()), projectID, runID)
	if err != nil {
		return fmt.Errorf("record model result: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: run %s", application.ErrNotFound, runID)
	}
	return nil
}

func (r *RuntimeRepository) RecordProposalOutcome(ctx context.Context, projectID, runID, checksum, outcome string) error {
	now := r.now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin proposal outcome: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE agent_runs SET proposal_checksum = ?, application_outcome = ?, updated_at = ? WHERE project_id = ? AND id = ?`, checksum, outcome, formatTime(now), projectID, runID)
	if err != nil {
		return fmt.Errorf("record proposal outcome: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: run %s", application.ErrNotFound, runID)
	}
	if outcome == "reconciliation_required" {
		if err := insertEvent(ctx, tx, projectID, "run.reconciliation_required", map[string]any{"run_id": runID, "proposal_checksum": checksum}, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit proposal outcome: %w", err)
	}
	return nil
}

func (r *RuntimeRepository) Claim(ctx context.Context, workerID string, now time.Time, leaseDuration time.Duration) (*jobs.Job, error) {
	if _, err := r.RecoverExpired(ctx, now); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	job, err := scanJob(tx.QueryRowContext(ctx, jobSelect+` WHERE state = 'queued' AND available_at <= ? ORDER BY available_at, created_at, id LIMIT 1`, formatTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	expires := now.Add(leaseDuration).UTC()
	job.State, job.Attempts, job.LeaseOwner, job.LeaseExpiresAt, job.UpdatedAt = jobs.Running, job.Attempts+1, workerID, &expires, now.UTC()
	result, err := tx.ExecContext(ctx, `UPDATE jobs SET state = ?, attempts = ?, lease_owner = ?, lease_expires_at = ?, updated_at = ? WHERE id = ? AND state = 'queued'`,
		job.State, job.Attempts, workerID, formatTime(expires), formatTime(job.UpdatedAt), job.ID)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, fmt.Errorf("%w: job was claimed concurrently", application.ErrConflict)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *RuntimeRepository) Renew(ctx context.Context, jobID, workerID string, now time.Time, leaseDuration time.Duration) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET lease_expires_at = ?, updated_at = ? WHERE id = ? AND state = 'running' AND lease_owner = ?`, formatTime(now.Add(leaseDuration)), formatTime(now), jobID, workerID)
	return requireOne(result, err, "renew job lease")
}

func (r *RuntimeRepository) Complete(ctx context.Context, jobID, workerID string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET state = 'completed', lease_owner = '', lease_expires_at = NULL, updated_at = ? WHERE id = ? AND state = 'running' AND lease_owner = ?`, formatTime(now), jobID, workerID)
	return requireOne(result, err, "complete job")
}

func (r *RuntimeRepository) Fail(ctx context.Context, jobID, workerID string, now, retryAt time.Time, cause error) (jobs.State, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	job, err := scanJob(tx.QueryRowContext(ctx, jobSelect+` WHERE id = ? AND state = 'running' AND lease_owner = ?`, jobID, workerID))
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: running job %s", application.ErrNotFound, jobID)
	}
	if err != nil {
		return "", err
	}
	next := jobs.Failed
	availableAt := job.AvailableAt
	if job.Attempts < job.MaxAttempts {
		next, availableAt = jobs.Queued, retryAt
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET state = ?, available_at = ?, lease_owner = '', lease_expires_at = NULL, last_error = ?, updated_at = ? WHERE id = ?`, next, formatTime(availableAt), message, formatTime(now), jobID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return next, nil
}

func (r *RuntimeRepository) FailTerminal(ctx context.Context, jobID, workerID string, now time.Time, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET state = 'failed', lease_owner = '', lease_expires_at = NULL, last_error = ?, updated_at = ? WHERE id = ? AND state = 'running' AND lease_owner = ?`, message, formatTime(now), jobID, workerID)
	return requireOne(result, err, "fail job")
}

func (r *RuntimeRepository) CancelRun(ctx context.Context, runID string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET state = 'cancelled', lease_owner = '', lease_expires_at = NULL, updated_at = ? WHERE run_id = ? AND state IN ('queued', 'running')`, formatTime(now), runID)
	return err
}

func (r *RuntimeRepository) RecoverExpired(ctx context.Context, now time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT jobs.id, jobs.project_id, jobs.run_id, jobs.attempts, jobs.max_attempts, agent_runs.state
		FROM jobs JOIN agent_runs ON agent_runs.id = jobs.run_id WHERE jobs.state = 'running' AND jobs.lease_expires_at < ?`, formatTime(now))
	if err != nil {
		return 0, err
	}
	type expired struct {
		id, projectID, runID  string
		attempts, maxAttempts int
		runState              domain.RunState
	}
	var expiredJobs []expired
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.projectID, &item.runID, &item.attempts, &item.maxAttempts, &item.runState); err != nil {
			rows.Close()
			return 0, err
		}
		expiredJobs = append(expiredJobs, item)
	}
	rows.Close()
	for _, item := range expiredJobs {
		if item.attempts < item.maxAttempts {
			_, err = tx.ExecContext(ctx, `UPDATE jobs SET state = 'queued', available_at = ?, lease_owner = '', lease_expires_at = NULL, last_error = 'lease expired', updated_at = ? WHERE id = ?`, formatTime(now), formatTime(now), item.id)
			if err == nil && !domain.RunTerminal(item.runState) {
				_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state = 'queued', error_code = '', error_message = '', updated_at = ? WHERE id = ?`, formatTime(now), item.runID)
			}
			if err == nil && !domain.RunTerminal(item.runState) {
				err = insertEvent(ctx, tx, item.projectID, "run.state_changed", map[string]any{"run_id": item.runID, "previous": item.runState, "state": domain.RunQueued, "reason": "lease_expired"}, now)
			}
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE jobs SET state = 'failed', lease_owner = '', lease_expires_at = NULL, last_error = 'lease expired after final attempt', updated_at = ? WHERE id = ?`, formatTime(now), item.id)
			if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET state = 'failed', error_code = 'lease_expired', error_message = 'job lease expired after final attempt', completed_at = ?, updated_at = ? WHERE id = ? AND state NOT IN ('completed', 'failed', 'cancelled')`, formatTime(now), formatTime(now), item.runID)
			}
			if err == nil {
				err = insertEvent(ctx, tx, item.projectID, "run.state_changed", map[string]any{"run_id": item.runID, "previous": item.runState, "state": domain.RunFailed, "error_code": "lease_expired"}, now)
			}
			if err == nil {
				err = insertEvent(ctx, tx, item.projectID, "run.failed", map[string]string{"run_id": item.runID, "error_code": "lease_expired"}, now)
			}
		}
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(expiredJobs)), nil
}

const runSelect = `SELECT id, project_id, role, state, task, base_revision, budget_json, usage_json, idempotency_key, request_checksum,
	prompt_version, response_schema_version, model_identifier, provider_request_id, selected_context_ids_json, allowed_tools_json,
	proposal_checksum, application_outcome, error_code, error_message, cancel_requested_at, created_at, started_at, completed_at, updated_at FROM agent_runs`

func scanRun(row scanner) (domain.AgentRun, error) {
	var run domain.AgentRun
	var budget, usage, contextIDs, allowedTools, createdAt, updatedAt string
	var cancelAt, startedAt, completedAt sql.NullString
	err := row.Scan(&run.ID, &run.ProjectID, &run.Role, &run.State, &run.Task, &run.BaseRevision, &budget, &usage, &run.IdempotencyKey, &run.RequestChecksum,
		&run.PromptVersion, &run.ResponseSchemaVersion, &run.ModelIdentifier, &run.ProviderRequestID, &contextIDs, &allowedTools,
		&run.ProposalChecksum, &run.ApplicationOutcome, &run.ErrorCode, &run.ErrorMessage, &cancelAt, &createdAt, &startedAt, &completedAt, &updatedAt)
	if err != nil {
		return domain.AgentRun{}, err
	}
	if err := json.Unmarshal([]byte(budget), &run.Budget); err != nil {
		return domain.AgentRun{}, err
	}
	if err := json.Unmarshal([]byte(usage), &run.Usage); err != nil {
		return domain.AgentRun{}, err
	}
	if err := json.Unmarshal([]byte(contextIDs), &run.SelectedContextIDs); err != nil {
		return domain.AgentRun{}, err
	}
	if err := json.Unmarshal([]byte(allowedTools), &run.AllowedTools); err != nil {
		return domain.AgentRun{}, err
	}
	if run.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.AgentRun{}, err
	}
	if run.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.AgentRun{}, err
	}
	if run.CancelRequestedAt, err = parseNullableTime(cancelAt); err != nil {
		return domain.AgentRun{}, err
	}
	if run.StartedAt, err = parseNullableTime(startedAt); err != nil {
		return domain.AgentRun{}, err
	}
	if run.CompletedAt, err = parseNullableTime(completedAt); err != nil {
		return domain.AgentRun{}, err
	}
	return run, nil
}

const jobSelect = `SELECT id, project_id, run_id, type, state, attempts, max_attempts, available_at, lease_owner, lease_expires_at, last_error, created_at, updated_at FROM jobs`

func scanJob(row scanner) (jobs.Job, error) {
	var job jobs.Job
	var availableAt, createdAt, updatedAt string
	var leaseExpiresAt sql.NullString
	if err := row.Scan(&job.ID, &job.ProjectID, &job.RunID, &job.Type, &job.State, &job.Attempts, &job.MaxAttempts, &availableAt, &job.LeaseOwner, &leaseExpiresAt, &job.LastError, &createdAt, &updatedAt); err != nil {
		return jobs.Job{}, err
	}
	var err error
	if job.AvailableAt, err = parseTime(availableAt); err != nil {
		return jobs.Job{}, err
	}
	if job.CreatedAt, err = parseTime(createdAt); err != nil {
		return jobs.Job{}, err
	}
	if job.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return jobs.Job{}, err
	}
	if job.LeaseExpiresAt, err = parseNullableTime(leaseExpiresAt); err != nil {
		return jobs.Job{}, err
	}
	return job, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func requireOne(result sql.Result, err error, operation string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("%w: %s target", application.ErrConflict, operation)
	}
	return nil
}

func insertRunStep(ctx context.Context, tx *sql.Tx, runID string, sequence int, state domain.RunState, summary string, occurredAt time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_run_steps (run_id, sequence, kind, state, summary, usage_json, started_at, completed_at) VALUES (?, ?, 'state', ?, ?, '{}', ?, ?)`,
		runID, sequence, state, summary, formatTime(occurredAt), formatTime(occurredAt))
	if err != nil {
		return fmt.Errorf("insert run step: %w", err)
	}
	return nil
}
