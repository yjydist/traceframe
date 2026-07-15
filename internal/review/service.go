package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

type Service struct {
	projects *application.ProjectService
	store    Store
	now      func() time.Time
}

func NewService(projects *application.ProjectService, store Store) *Service {
	return &Service{projects: projects, store: store, now: time.Now}
}

func (s *Service) SubmitFindings(ctx context.Context, projectID, runID string, baseRevision int64, drafts []FindingDraft) error {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return err
	}
	if snapshot.Project.Revision != baseRevision {
		return fmt.Errorf("%w: review revision %d, current revision is %d", application.ErrConflict, baseRevision, snapshot.Project.Revision)
	}
	if len(drafts) == 0 || len(drafts) > 100 {
		return fmt.Errorf("%w: critic must submit between 1 and 100 findings", domain.ErrInvalid)
	}
	entities := make(map[string]struct{}, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		entities[entity.ID] = struct{}{}
	}
	now := s.now().UTC()
	findings := make([]Finding, len(drafts))
	seen := make(map[string]struct{}, len(drafts))
	for index, draft := range drafts {
		if err := validateDraft(draft, entities); err != nil {
			return fmt.Errorf("finding %d: %w", index, err)
		}
		if _, exists := seen[draft.ID]; exists {
			return fmt.Errorf("%w: duplicate finding id %s", domain.ErrInvalid, draft.ID)
		}
		seen[draft.ID] = struct{}{}
		findings[index] = Finding{ID: draft.ID, ProjectID: projectID, RunID: runID, ProjectRevision: baseRevision, Severity: draft.Severity, Category: strings.TrimSpace(draft.Category), AffectedEntityIDs: append([]string{}, draft.AffectedEntityIDs...), Claim: strings.TrimSpace(draft.Claim), Evidence: strings.TrimSpace(draft.Evidence), RecommendedResolution: strings.TrimSpace(draft.RecommendedResolution), Status: FindingOpen, CounterEvidenceRefs: []string{}, CreatedAt: now, UpdatedAt: now}
	}
	return s.store.CreateFindings(ctx, projectID, runID, baseRevision, findings)
}

func validateDraft(draft FindingDraft, entities map[string]struct{}) error {
	if strings.TrimSpace(draft.ID) == "" || strings.TrimSpace(draft.Category) == "" || strings.TrimSpace(draft.Claim) == "" || strings.TrimSpace(draft.Evidence) == "" || strings.TrimSpace(draft.RecommendedResolution) == "" {
		return fmt.Errorf("%w: finding id, category, claim, evidence, and recommended_resolution are required", domain.ErrInvalid)
	}
	switch draft.Severity {
	case SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityBlocking:
	default:
		return fmt.Errorf("%w: unsupported finding severity %q", domain.ErrInvalid, draft.Severity)
	}
	for _, id := range draft.AffectedEntityIDs {
		if _, ok := entities[id]; !ok {
			return fmt.Errorf("%w: affected entity %s", application.ErrNotFound, id)
		}
	}
	return nil
}

func (s *Service) ListFindings(ctx context.Context, projectID string) ([]Finding, error) {
	if _, err := s.projects.Snapshot(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListFindings(ctx, projectID)
}

func (s *Service) ResolveFinding(ctx context.Context, projectID, findingID string, resolution Resolution) (Finding, error) {
	resolution.Rationale = strings.TrimSpace(resolution.Rationale)
	if resolution.ExpectedRevision < 1 || resolution.Rationale == "" {
		return Finding{}, fmt.Errorf("%w: expected_revision and rationale are required", domain.ErrInvalid)
	}
	switch resolution.Status {
	case FindingResolved, FindingDismissed, FindingRiskAccepted:
	default:
		return Finding{}, fmt.Errorf("%w: unsupported finding resolution %q", domain.ErrInvalid, resolution.Status)
	}
	if len(resolution.CounterEvidenceRefs) > 0 {
		snapshot, err := s.projects.Snapshot(ctx, projectID)
		if err != nil {
			return Finding{}, err
		}
		for _, reference := range resolution.CounterEvidenceRefs {
			valid := false
			for _, entity := range snapshot.Entities {
				if entity.ID == reference && entity.Kind == domain.KindEvidence && entity.Freshness == domain.FreshnessCurrent {
					valid = true
					break
				}
			}
			if !valid {
				return Finding{}, fmt.Errorf("%w: counter-evidence %s", application.ErrNotFound, reference)
			}
		}
	}
	return s.store.ResolveFinding(ctx, projectID, findingID, resolution, "user")
}

func (s *Service) Readiness(ctx context.Context, projectID string) (Readiness, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return Readiness{}, err
	}
	approvals, err := s.store.ListApprovals(ctx, projectID)
	if err != nil {
		return Readiness{}, err
	}
	findings, err := s.store.ListFindings(ctx, projectID)
	if err != nil {
		return Readiness{}, err
	}
	checks := []ReadinessCheck{
		{Code: "confirmed_goal", Passed: hasEntity(snapshot, domain.KindGoal, domain.EntityConfirmed), Message: "At least one goal is confirmed."},
		{Code: "scope_and_non_goals", Passed: hasNonGoal(snapshot), Message: "Scope and non-goals are recorded."},
		{Code: "requirement_traceability", Passed: requirementsTraced(snapshot), Message: "Every confirmed requirement traces to a goal or scenario."},
		{Code: "must_verification", Passed: mustRequirementsVerified(snapshot), Message: "Every must requirement has current verification."},
		{Code: "architectural_approvals", Passed: architecturalDecisionsApproved(snapshot, approvals), Message: "Every current architectural decision is approved at its exact entity revision."},
		{Code: "blocking_questions", Passed: noBlockingQuestions(snapshot), Message: "No blocking question remains unresolved."},
		{Code: "blocking_findings", Passed: blockingFindingsResolved(findings), Message: "Every blocking finding is resolved or dismissed with counter-evidence."},
		{Code: "residual_risk_acceptance", Passed: highRisksAccepted(snapshot, approvals), Message: "Every high or critical residual risk is explicitly accepted."},
		{Code: "delivery_completion", Passed: slicesComplete(snapshot), Message: "Every work slice has completion conditions and verification references."},
		{Code: "active_conflicts", Passed: noActiveConflicts(snapshot), Message: "No active model contradiction remains."},
	}
	ready := snapshot.Project.Stage == domain.StageReview
	blockers := make([]string, 0)
	if !ready {
		blockers = append(blockers, "stage_review")
	}
	for _, check := range checks {
		if !check.Passed {
			ready = false
			blockers = append(blockers, check.Code)
		}
	}
	return Readiness{ProjectID: projectID, ProjectRevision: snapshot.Project.Revision, Ready: ready, Checks: checks, Blockers: blockers, UpdatedAt: s.now().UTC()}, nil
}

func (s *Service) CreateBaseline(ctx context.Context, projectID string, request BaselineRequest) (Baseline, error) {
	if !request.Approve || strings.TrimSpace(request.Rationale) == "" {
		return Baseline{}, fmt.Errorf("%w: explicit baseline approval and rationale are required", domain.ErrInvalid)
	}
	readiness, err := s.Readiness(ctx, projectID)
	if err != nil {
		return Baseline{}, err
	}
	if readiness.ProjectRevision != request.ExpectedRevision {
		return Baseline{}, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, request.ExpectedRevision, readiness.ProjectRevision)
	}
	if !readiness.Ready {
		return Baseline{}, fmt.Errorf("%w: readiness is blocked by %s", domain.ErrInvalid, strings.Join(readiness.Blockers, ", "))
	}
	now := s.now().UTC()
	baseline := Baseline{ID: domain.NewID("baseline"), ProjectID: projectID, ProjectRevision: request.ExpectedRevision, ApprovalActor: "user", ApprovalRationale: strings.TrimSpace(request.Rationale), ApprovedAt: now, CreatedAt: now}
	baseline, _, err = s.store.CreateBaseline(ctx, baseline, request.ExpectedRevision)
	if err != nil {
		return Baseline{}, err
	}
	gateChecks := make([]application.StageGateCheck, len(readiness.Checks))
	for index, check := range readiness.Checks {
		gateChecks[index] = application.StageGateCheck{Code: check.Code, Passed: check.Passed, Message: check.Message}
	}
	if _, err := s.projects.ChangeStage(ctx, projectID, request.ExpectedRevision, application.StageTransition{Next: domain.StageReady, Actor: "user", GateChecks: gateChecks, Unresolved: readiness.Blockers, Reason: "User approved the exact baseline revision.", ApprovalReference: baseline.ID}); err != nil {
		return Baseline{}, err
	}
	return baseline, nil
}

func (s *Service) ListBaselines(ctx context.Context, projectID string) ([]Baseline, error) {
	if _, err := s.projects.Snapshot(ctx, projectID); err != nil {
		return nil, err
	}
	return s.store.ListBaselines(ctx, projectID)
}

func hasEntity(snapshot domain.Snapshot, kind domain.EntityKind, status domain.EntityStatus) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind == kind && entity.Status == status && entity.Freshness == domain.FreshnessCurrent {
			return true
		}
	}
	return false
}

func hasNonGoal(snapshot domain.Snapshot) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindScopeItem || entity.Status != domain.EntityConfirmed || entity.Freshness != domain.FreshnessCurrent {
			continue
		}
		var body map[string]any
		if json.Unmarshal(entity.Body, &body) == nil && body["disposition"] == "out_of_scope" {
			return true
		}
	}
	return false
}

func entityMap(snapshot domain.Snapshot) map[string]domain.Entity {
	result := make(map[string]domain.Entity, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		result[entity.ID] = entity
	}
	return result
}

func requirementsTraced(snapshot domain.Snapshot) bool {
	entities := entityMap(snapshot)
	found := false
	for _, requirement := range snapshot.Entities {
		if requirement.Kind != domain.KindRequirement || requirement.Status != domain.EntityConfirmed || requirement.Freshness != domain.FreshnessCurrent {
			continue
		}
		found = true
		linked := false
		for _, relation := range snapshot.Relations {
			target := entities[relation.ToID]
			if relation.FromID == requirement.ID && relation.Type == domain.RelationSatisfies && (target.Kind == domain.KindGoal || target.Kind == domain.KindScenario) && target.Status == domain.EntityConfirmed && target.Freshness == domain.FreshnessCurrent {
				linked = true
			}
		}
		if !linked {
			return false
		}
	}
	return found
}

func mustRequirementsVerified(snapshot domain.Snapshot) bool {
	entities := entityMap(snapshot)
	for _, requirement := range snapshot.Entities {
		if requirement.Kind != domain.KindRequirement || requirement.Status != domain.EntityConfirmed || requirement.Freshness != domain.FreshnessCurrent {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(requirement.Body, &body)
		if body["priority"] != "must" {
			continue
		}
		verified := false
		for _, relation := range snapshot.Relations {
			verification := entities[relation.FromID]
			if relation.ToID == requirement.ID && relation.Type == domain.RelationVerifies && verification.Kind == domain.KindVerification && verification.Status == domain.EntityConfirmed && verification.Freshness == domain.FreshnessCurrent {
				verified = true
			}
		}
		if !verified {
			return false
		}
	}
	return true
}

func exactApproval(approvals []domain.Approval, entity domain.Entity) bool {
	for _, approval := range approvals {
		if approval.SubjectID == entity.ID && approval.SubjectRevision == entity.Revision && approval.Status == domain.ApprovalApproved {
			return true
		}
	}
	return false
}

func architecturalDecisionsApproved(snapshot domain.Snapshot, approvals []domain.Approval) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindDecision || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded || entity.Freshness != domain.FreshnessCurrent {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(entity.Body, &body)
		if (body["significance"] == "architectural" || body["approval_required"] == true) && !exactApproval(approvals, entity) {
			return false
		}
	}
	return true
}

func noBlockingQuestions(snapshot domain.Snapshot) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindQuestion || entity.Status == domain.EntityConfirmed || entity.Status == domain.EntityRejected {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(entity.Body, &body)
		if body["blocking"] == true && body["disposition"] != "deferred" && body["disposition"] != "answered" && body["disposition"] != "rejected" {
			return false
		}
	}
	return true
}

func blockingFindingsResolved(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity != SeverityBlocking {
			continue
		}
		if finding.Status == FindingResolved {
			continue
		}
		if finding.Status == FindingDismissed && len(finding.CounterEvidenceRefs) > 0 {
			continue
		}
		return false
	}
	return true
}

func highRisksAccepted(snapshot domain.Snapshot, approvals []domain.Approval) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindRisk || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded || entity.Freshness != domain.FreshnessCurrent {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(entity.Body, &body)
		residual := strings.ToLower(fmt.Sprint(body["residual_risk"]))
		if (residual == "high" || residual == "critical" || residual == "4" || residual == "5") && !exactApproval(approvals, entity) {
			return false
		}
	}
	return true
}

func slicesComplete(snapshot domain.Snapshot) bool {
	found := false
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindWorkSlice || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded || entity.Freshness != domain.FreshnessCurrent {
			continue
		}
		found = true
		var body struct {
			VerificationRefs     []string `json:"verification_refs"`
			CompletionConditions []any    `json:"completion_conditions"`
		}
		if json.Unmarshal(entity.Body, &body) != nil || len(body.VerificationRefs) == 0 || len(body.CompletionConditions) == 0 {
			return false
		}
	}
	return found
}

func noActiveConflicts(snapshot domain.Snapshot) bool {
	entities := entityMap(snapshot)
	for _, relation := range snapshot.Relations {
		if relation.Type != domain.RelationConflictsWith {
			continue
		}
		from, fromOK := entities[relation.FromID]
		to, toOK := entities[relation.ToID]
		if fromOK && toOK && activeEntity(from) && activeEntity(to) {
			return false
		}
	}
	return true
}

func activeEntity(entity domain.Entity) bool {
	return (entity.Status == domain.EntityProposed || entity.Status == domain.EntityConfirmed || entity.Status == domain.EntityUnresolved) && entity.Freshness == domain.FreshnessCurrent
}
