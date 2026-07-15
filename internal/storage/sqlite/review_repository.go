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
	"github.com/yjydist/traceframe/internal/review"
)

type ReviewRepository struct{ db *sql.DB }

func NewReviewRepository(db *sql.DB) *ReviewRepository { return &ReviewRepository{db: db} }

func (r *ReviewRepository) CreateFindings(ctx context.Context, projectID, runID string, projectRevision int64, findings []review.Finding) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin review findings: %w", err)
	}
	defer tx.Rollback()
	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE id = ?`, projectID).Scan(&currentRevision); err != nil {
		return fmt.Errorf("load project for findings: %w", err)
	}
	if currentRevision != projectRevision {
		return fmt.Errorf("%w: review revision %d, current revision is %d", application.ErrConflict, projectRevision, currentRevision)
	}
	for _, finding := range findings {
		affected, _ := json.Marshal(finding.AffectedEntityIDs)
		counter, _ := json.Marshal(finding.CounterEvidenceRefs)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO review_findings (id, project_id, run_id, project_revision, severity, category, affected_entity_ids_json, claim, evidence, recommended_resolution, status, resolution_rationale, counter_evidence_refs_json, resolved_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			finding.ID, projectID, runID, projectRevision, finding.Severity, finding.Category, string(affected), finding.Claim, finding.Evidence, finding.RecommendedResolution, finding.Status, finding.ResolutionRationale, string(counter), finding.ResolvedBy, formatTime(finding.CreatedAt), formatTime(finding.UpdatedAt)); err != nil {
			if isConstraintError(err) {
				return fmt.Errorf("%w: review finding %s already exists", application.ErrConflict, finding.ID)
			}
			return fmt.Errorf("insert review finding: %w", err)
		}
		if err := insertEvent(ctx, tx, projectID, "review.finding_created", map[string]any{"finding_id": finding.ID, "run_id": runID, "severity": finding.Severity, "model_revision": projectRevision}, finding.CreatedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit review findings: %w", err)
	}
	return nil
}

func (r *ReviewRepository) ListFindings(ctx context.Context, projectID string) ([]review.Finding, error) {
	rows, err := r.db.QueryContext(ctx, findingSelect+` WHERE project_id = ? ORDER BY CASE severity WHEN 'blocking' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, created_at, id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list review findings: %w", err)
	}
	defer rows.Close()
	findings := make([]review.Finding, 0)
	for rows.Next() {
		finding, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate review findings: %w", err)
	}
	return findings, nil
}

func (r *ReviewRepository) ResolveFinding(ctx context.Context, projectID, findingID string, resolution review.Resolution, actor string) (review.Finding, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return review.Finding{}, fmt.Errorf("begin finding resolution: %w", err)
	}
	defer tx.Rollback()
	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE id = ?`, projectID).Scan(&currentRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return review.Finding{}, fmt.Errorf("%w: project %s", application.ErrNotFound, projectID)
		}
		return review.Finding{}, fmt.Errorf("load project for finding resolution: %w", err)
	}
	if currentRevision != resolution.ExpectedRevision {
		return review.Finding{}, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, resolution.ExpectedRevision, currentRevision)
	}
	finding, err := scanFinding(tx.QueryRowContext(ctx, findingSelect+` WHERE project_id = ? AND id = ?`, projectID, findingID))
	if err != nil {
		return review.Finding{}, err
	}
	if finding.Status != review.FindingOpen {
		return review.Finding{}, fmt.Errorf("%w: finding %s is %s", domain.ErrInvalid, findingID, finding.Status)
	}
	if finding.Severity == review.SeverityBlocking && resolution.Status == review.FindingRiskAccepted {
		return review.Finding{}, fmt.Errorf("%w: blocking findings cannot be risk accepted", domain.ErrInvalid)
	}
	if finding.Severity == review.SeverityBlocking && resolution.Status == review.FindingDismissed && len(resolution.CounterEvidenceRefs) == 0 {
		return review.Finding{}, fmt.Errorf("%w: dismissing a blocking finding requires counter_evidence_refs", domain.ErrInvalid)
	}
	now := time.Now().UTC()
	finding.Status, finding.ResolutionRationale, finding.CounterEvidenceRefs, finding.ResolvedBy, finding.ResolvedAt, finding.UpdatedAt = resolution.Status, strings.TrimSpace(resolution.Rationale), append([]string{}, resolution.CounterEvidenceRefs...), strings.TrimSpace(actor), &now, now
	counter, _ := json.Marshal(finding.CounterEvidenceRefs)
	if _, err := tx.ExecContext(ctx, `UPDATE review_findings SET status = ?, resolution_rationale = ?, counter_evidence_refs_json = ?, resolved_by = ?, resolved_at = ?, updated_at = ? WHERE id = ?`, finding.Status, finding.ResolutionRationale, string(counter), finding.ResolvedBy, formatTime(now), formatTime(now), finding.ID); err != nil {
		return review.Finding{}, fmt.Errorf("resolve review finding: %w", err)
	}
	if err := insertEvent(ctx, tx, projectID, "review.finding_resolved", map[string]any{"finding_id": finding.ID, "status": finding.Status, "actor": finding.ResolvedBy, "model_revision": currentRevision, "counter_evidence_refs": finding.CounterEvidenceRefs}, now); err != nil {
		return review.Finding{}, err
	}
	if err := tx.Commit(); err != nil {
		return review.Finding{}, fmt.Errorf("commit finding resolution: %w", err)
	}
	return finding, nil
}

func (r *ReviewRepository) ListApprovals(ctx context.Context, projectID string) ([]domain.Approval, error) {
	rows, err := r.db.QueryContext(ctx, approvalSelect+` WHERE project_id = ? ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list approvals for review: %w", err)
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
	return approvals, rows.Err()
}

func (r *ReviewRepository) CreateBaseline(ctx context.Context, baseline review.Baseline, expectedRevision int64) (review.Baseline, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return review.Baseline{}, false, fmt.Errorf("begin baseline: %w", err)
	}
	defer tx.Rollback()
	var currentRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE id = ?`, baseline.ProjectID).Scan(&currentRevision); err != nil {
		return review.Baseline{}, false, fmt.Errorf("load project for baseline: %w", err)
	}
	if currentRevision != expectedRevision || baseline.ProjectRevision != expectedRevision {
		return review.Baseline{}, false, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, expectedRevision, currentRevision)
	}
	var snapshot string
	if err := tx.QueryRowContext(ctx, `SELECT checksum, snapshot_json FROM project_revisions WHERE project_id = ? AND revision = ?`, baseline.ProjectID, expectedRevision).Scan(&baseline.Checksum, &snapshot); err != nil {
		return review.Baseline{}, false, fmt.Errorf("load baseline revision: %w", err)
	}
	baseline.Snapshot = json.RawMessage(snapshot)
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO baselines (id, project_id, project_revision, checksum, snapshot_json, approval_actor, approval_rationale, approved_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, baseline.ID, baseline.ProjectID, baseline.ProjectRevision, baseline.Checksum, string(baseline.Snapshot), baseline.ApprovalActor, baseline.ApprovalRationale, formatTime(baseline.ApprovedAt), formatTime(baseline.CreatedAt))
	if err != nil {
		return review.Baseline{}, false, fmt.Errorf("insert baseline: %w", err)
	}
	created, _ := result.RowsAffected()
	if created == 0 {
		existing, err := scanBaseline(tx.QueryRowContext(ctx, baselineSelect+` WHERE project_id = ? AND project_revision = ?`, baseline.ProjectID, expectedRevision))
		return existing, false, err
	}
	if err := insertEvent(ctx, tx, baseline.ProjectID, "baseline.created", map[string]any{"baseline_id": baseline.ID, "model_revision": baseline.ProjectRevision, "checksum": baseline.Checksum, "approval_actor": baseline.ApprovalActor}, baseline.CreatedAt); err != nil {
		return review.Baseline{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return review.Baseline{}, false, fmt.Errorf("commit baseline: %w", err)
	}
	return baseline, true, nil
}

func (r *ReviewRepository) ListBaselines(ctx context.Context, projectID string) ([]review.Baseline, error) {
	rows, err := r.db.QueryContext(ctx, baselineSelect+` WHERE project_id = ? ORDER BY project_revision DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list baselines: %w", err)
	}
	defer rows.Close()
	baselines := make([]review.Baseline, 0)
	for rows.Next() {
		baseline, err := scanBaseline(rows)
		if err != nil {
			return nil, err
		}
		baselines = append(baselines, baseline)
	}
	return baselines, rows.Err()
}

const findingSelect = `SELECT id, project_id, run_id, project_revision, severity, category, affected_entity_ids_json, claim, evidence, recommended_resolution, status, resolution_rationale, counter_evidence_refs_json, resolved_by, created_at, resolved_at, updated_at FROM review_findings`

func scanFinding(scanner approvalScanner) (review.Finding, error) {
	var finding review.Finding
	var affected, counter, createdAt, updatedAt string
	var resolvedAt sql.NullString
	if err := scanner.Scan(&finding.ID, &finding.ProjectID, &finding.RunID, &finding.ProjectRevision, &finding.Severity, &finding.Category, &affected, &finding.Claim, &finding.Evidence, &finding.RecommendedResolution, &finding.Status, &finding.ResolutionRationale, &counter, &finding.ResolvedBy, &createdAt, &resolvedAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return review.Finding{}, fmt.Errorf("%w: review finding", application.ErrNotFound)
		}
		return review.Finding{}, fmt.Errorf("scan review finding: %w", err)
	}
	if err := json.Unmarshal([]byte(affected), &finding.AffectedEntityIDs); err != nil {
		return review.Finding{}, fmt.Errorf("decode affected entities: %w", err)
	}
	if err := json.Unmarshal([]byte(counter), &finding.CounterEvidenceRefs); err != nil {
		return review.Finding{}, fmt.Errorf("decode counter evidence: %w", err)
	}
	var err error
	if finding.CreatedAt, err = parseTime(createdAt); err != nil {
		return review.Finding{}, err
	}
	if finding.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return review.Finding{}, err
	}
	if resolvedAt.Valid {
		value, err := parseTime(resolvedAt.String)
		if err != nil {
			return review.Finding{}, err
		}
		finding.ResolvedAt = &value
	}
	return finding, nil
}

const baselineSelect = `SELECT id, project_id, project_revision, checksum, snapshot_json, approval_actor, approval_rationale, approved_at, created_at FROM baselines`

func scanBaseline(scanner approvalScanner) (review.Baseline, error) {
	var baseline review.Baseline
	var snapshot, approvedAt, createdAt string
	if err := scanner.Scan(&baseline.ID, &baseline.ProjectID, &baseline.ProjectRevision, &baseline.Checksum, &snapshot, &baseline.ApprovalActor, &baseline.ApprovalRationale, &approvedAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return review.Baseline{}, fmt.Errorf("%w: baseline", application.ErrNotFound)
		}
		return review.Baseline{}, fmt.Errorf("scan baseline: %w", err)
	}
	baseline.Snapshot = json.RawMessage(snapshot)
	var err error
	if baseline.ApprovedAt, err = parseTime(approvedAt); err != nil {
		return review.Baseline{}, err
	}
	if baseline.CreatedAt, err = parseTime(createdAt); err != nil {
		return review.Baseline{}, err
	}
	return baseline, nil
}
