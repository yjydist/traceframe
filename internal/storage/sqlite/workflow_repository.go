package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/workflow"
)

type WorkflowRepository struct{ db *sql.DB }

func NewWorkflowRepository(db *sql.DB) *WorkflowRepository { return &WorkflowRepository{db: db} }

func (r *WorkflowRepository) LoadAssessment(ctx context.Context, projectID string) (workflow.Assessment, bool, error) {
	var payload string
	err := r.db.QueryRowContext(ctx, `SELECT assessment_json FROM assessments WHERE project_id = ?`, projectID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return workflow.Assessment{}, false, nil
	}
	if err != nil {
		return workflow.Assessment{}, false, fmt.Errorf("load assessment: %w", err)
	}
	var assessment workflow.Assessment
	if err := json.Unmarshal([]byte(payload), &assessment); err != nil {
		return workflow.Assessment{}, false, fmt.Errorf("decode assessment: %w", err)
	}
	return assessment, true, nil
}

func (r *WorkflowRepository) SaveAssessment(ctx context.Context, assessment workflow.Assessment) error {
	data, err := json.Marshal(assessment)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO assessments (project_id, assessment_json, project_revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(project_id) DO UPDATE SET assessment_json = excluded.assessment_json, project_revision = excluded.project_revision, updated_at = excluded.updated_at`,
		assessment.ProjectID, string(data), assessment.ProjectRevision, formatTime(assessment.UpdatedAt), formatTime(assessment.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save assessment: %w", err)
	}
	return nil
}

func (r *WorkflowRepository) SaveState(ctx context.Context, state workflow.State) error {
	gate, err := json.Marshal(map[string]any{"passed": state.GatePassed, "checks": state.Checks, "blockers": state.Blockers})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO workflow_states (project_id, stage, gate_json, reason, project_revision, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(project_id) DO UPDATE SET stage = excluded.stage, gate_json = excluded.gate_json, reason = excluded.reason, project_revision = excluded.project_revision, updated_at = excluded.updated_at`,
		state.ProjectID, state.Stage, string(gate), state.Reason, state.ProjectRevision, formatTime(state.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save workflow state: %w", err)
	}
	return nil
}

func (r *WorkflowRepository) RecordBlocked(ctx context.Context, projectID string, state workflow.State) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertEvent(ctx, tx, projectID, "workflow.blocked", map[string]any{"stage": state.Stage, "model_revision": state.ProjectRevision, "checks": state.Checks, "blockers": state.Blockers}, state.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *WorkflowRepository) RequestApproval(ctx context.Context, approval domain.Approval) (domain.Approval, bool, error) {
	if err := domain.ValidateApproval(approval); err != nil {
		return domain.Approval{}, false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Approval{}, false, fmt.Errorf("begin approval request: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO approvals (id, project_id, subject_id, subject_revision, project_revision, status, requested_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, approval.ID, approval.ProjectID, approval.SubjectID, approval.SubjectRevision, approval.ProjectRevision,
		approval.Status, approval.RequestedBy, formatTime(approval.CreatedAt), formatTime(approval.UpdatedAt))
	if err != nil {
		return domain.Approval{}, false, fmt.Errorf("insert approval request: %w", err)
	}
	created, _ := result.RowsAffected()
	if created == 1 {
		if err := insertEvent(ctx, tx, approval.ProjectID, "approval.requested", map[string]any{"approval_id": approval.ID, "subject_id": approval.SubjectID, "subject_revision": approval.SubjectRevision, "model_revision": approval.ProjectRevision}, approval.CreatedAt); err != nil {
			return domain.Approval{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Approval{}, false, fmt.Errorf("commit approval request: %w", err)
	}
	if created == 0 {
		existing, err := r.getApproval(ctx, approval.ProjectID, approval.SubjectID, approval.SubjectRevision)
		return existing, false, err
	}
	return approval, true, nil
}

func (r *WorkflowRepository) ListApprovals(ctx context.Context, projectID string) ([]domain.Approval, error) {
	rows, err := r.db.QueryContext(ctx, approvalSelect+` WHERE project_id = ? ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer rows.Close()
	approvals := make([]domain.Approval, 0)
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approvals: %w", err)
	}
	return approvals, nil
}

func (r *WorkflowRepository) ResolveApproval(ctx context.Context, projectID, approvalID string, expectedProjectRevision int64, status domain.ApprovalStatus, actor, rationale string) (domain.Approval, error) {
	if status != domain.ApprovalApproved && status != domain.ApprovalRejected {
		return domain.Approval{}, fmt.Errorf("%w: approval resolution must be approved or rejected", domain.ErrInvalid)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Approval{}, fmt.Errorf("begin approval resolution: %w", err)
	}
	defer tx.Rollback()
	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE id = ?`, projectID).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Approval{}, fmt.Errorf("%w: project %s", application.ErrNotFound, projectID)
		}
		return domain.Approval{}, fmt.Errorf("load project for approval: %w", err)
	}
	if currentRevision != expectedProjectRevision {
		return domain.Approval{}, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, expectedProjectRevision, currentRevision)
	}
	approval, err := scanApproval(tx.QueryRowContext(ctx, approvalSelect+` WHERE project_id = ? AND id = ?`, projectID, approvalID))
	if err != nil {
		return domain.Approval{}, err
	}
	if approval.Status != domain.ApprovalPending {
		return domain.Approval{}, fmt.Errorf("%w: approval %s is %s", domain.ErrInvalid, approvalID, approval.Status)
	}
	var subjectRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM entities WHERE project_id = ? AND id = ?`, projectID, approval.SubjectID).Scan(&subjectRevision); err != nil {
		return domain.Approval{}, fmt.Errorf("load approval subject: %w", err)
	}
	if subjectRevision != approval.SubjectRevision {
		return domain.Approval{}, fmt.Errorf("%w: approval subject revision %d is no longer current", application.ErrConflict, approval.SubjectRevision)
	}
	now := time.Now().UTC()
	approval.Status, approval.ProjectRevision, approval.ResolvedBy, approval.Rationale, approval.ResolvedAt, approval.UpdatedAt = status, currentRevision, strings.TrimSpace(actor), strings.TrimSpace(rationale), &now, now
	if _, err := tx.ExecContext(ctx, `UPDATE approvals SET status = ?, project_revision = ?, resolved_by = ?, rationale = ?, resolved_at = ?, updated_at = ? WHERE id = ?`, status, currentRevision, approval.ResolvedBy, approval.Rationale, formatTime(now), formatTime(now), approval.ID); err != nil {
		return domain.Approval{}, fmt.Errorf("resolve approval: %w", err)
	}
	if err := insertEvent(ctx, tx, projectID, "approval.resolved", map[string]any{"approval_id": approval.ID, "subject_id": approval.SubjectID, "subject_revision": approval.SubjectRevision, "model_revision": currentRevision, "status": status, "actor": approval.ResolvedBy, "rationale": approval.Rationale}, now); err != nil {
		return domain.Approval{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Approval{}, fmt.Errorf("commit approval resolution: %w", err)
	}
	return approval, nil
}

const approvalSelect = `SELECT id, project_id, subject_id, subject_revision, project_revision, status, requested_by, resolved_by, rationale, created_at, resolved_at, updated_at FROM approvals`

type approvalScanner interface{ Scan(...any) error }

func scanApproval(scanner approvalScanner) (domain.Approval, error) {
	var approval domain.Approval
	var createdAt, updatedAt string
	var resolvedAt sql.NullString
	if err := scanner.Scan(&approval.ID, &approval.ProjectID, &approval.SubjectID, &approval.SubjectRevision, &approval.ProjectRevision, &approval.Status, &approval.RequestedBy, &approval.ResolvedBy, &approval.Rationale, &createdAt, &resolvedAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Approval{}, fmt.Errorf("%w: approval", application.ErrNotFound)
		}
		return domain.Approval{}, fmt.Errorf("scan approval: %w", err)
	}
	var err error
	if approval.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Approval{}, err
	}
	if approval.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Approval{}, err
	}
	if resolvedAt.Valid {
		value, err := parseTime(resolvedAt.String)
		if err != nil {
			return domain.Approval{}, err
		}
		approval.ResolvedAt = &value
	}
	return approval, nil
}

func (r *WorkflowRepository) getApproval(ctx context.Context, projectID, subjectID string, subjectRevision int64) (domain.Approval, error) {
	return scanApproval(r.db.QueryRowContext(ctx, approvalSelect+` WHERE project_id = ? AND subject_id = ? AND subject_revision = ?`, projectID, subjectID, subjectRevision))
}
