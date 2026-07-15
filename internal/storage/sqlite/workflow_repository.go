package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/yjydist/traceframe/internal/workflow"
)

type WorkflowRepository struct{ db *sql.DB }

func NewWorkflowRepository(db *sql.DB) *WorkflowRepository { return &WorkflowRepository{db: db} }

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
