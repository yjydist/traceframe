package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
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

func (s *Service) Get(ctx context.Context, projectID string) (State, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	assessment := assess(snapshot, s.now().UTC())
	if previous, ok, loadErr := s.store.LoadAssessment(ctx, projectID); loadErr != nil {
		return State{}, loadErr
	} else if ok && previous.Corrected {
		assessment.Criticality = previous.Criticality
		assessment.ActiveConcerns = append([]string{}, previous.ActiveConcerns...)
		assessment.Corrected = true
	}
	if err := s.store.SaveAssessment(ctx, assessment); err != nil {
		return State{}, err
	}
	if err := s.ensureDecisionApprovals(ctx, snapshot, "workflow"); err != nil {
		return State{}, err
	}
	approvals, err := s.store.ListApprovals(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	state := evaluate(snapshot, assessment, approvals, s.now().UTC())
	if err := s.store.SaveState(ctx, state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Service) CorrectAssessment(ctx context.Context, projectID string, correction AssessmentCorrection) (Assessment, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return Assessment{}, err
	}
	if snapshot.Project.Revision != correction.ExpectedRevision {
		return Assessment{}, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, correction.ExpectedRevision, snapshot.Project.Revision)
	}
	criticality := strings.TrimSpace(correction.Criticality)
	if criticality != "low" && criticality != "medium" && criticality != "high" {
		return Assessment{}, fmt.Errorf("%w: criticality must be low, medium, or high", domain.ErrInvalid)
	}
	concerns, err := normalizeConcerns(correction.ActiveConcerns)
	if err != nil {
		return Assessment{}, err
	}
	assessment := assess(snapshot, s.now().UTC())
	assessment.Criticality, assessment.ActiveConcerns, assessment.Corrected = criticality, concerns, true
	if err := s.store.SaveAssessment(ctx, assessment); err != nil {
		return Assessment{}, err
	}
	return assessment, nil
}

func (s *Service) ListApprovals(ctx context.Context, projectID string) ([]domain.Approval, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureDecisionApprovals(ctx, snapshot, "workflow"); err != nil {
		return nil, err
	}
	return s.store.ListApprovals(ctx, projectID)
}

func (s *Service) ResolveApproval(ctx context.Context, projectID, approvalID string, resolution ApprovalResolution, approve bool) (domain.Approval, error) {
	status := domain.ApprovalRejected
	if approve {
		status = domain.ApprovalApproved
	}
	return s.store.ResolveApproval(ctx, projectID, approvalID, resolution.ExpectedRevision, status, "user", resolution.Rationale)
}

func (s *Service) EnsureDecisionApprovals(ctx context.Context, projectID, requestedBy string) error {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return err
	}
	return s.ensureDecisionApprovals(ctx, snapshot, requestedBy)
}

func (s *Service) ensureDecisionApprovals(ctx context.Context, snapshot domain.Snapshot, requestedBy string) error {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindDecision || entity.Freshness != domain.FreshnessCurrent || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded || !decisionRequiresApproval(entity) {
			continue
		}
		now := s.now().UTC()
		approval := domain.Approval{ID: domain.NewID("approval"), ProjectID: snapshot.Project.ID, SubjectID: entity.ID, SubjectRevision: entity.Revision, ProjectRevision: snapshot.Project.Revision, Status: domain.ApprovalPending, RequestedBy: requestedBy, CreatedAt: now, UpdatedAt: now}
		if _, _, err := s.store.RequestApproval(ctx, approval); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Continue(ctx context.Context, projectID string, expectedRevision int64) (State, error) {
	state, err := s.Get(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	if state.ProjectRevision != expectedRevision {
		return State{}, fmt.Errorf("%w: expected project revision %d, current revision is %d", application.ErrConflict, expectedRevision, state.ProjectRevision)
	}
	if !state.GatePassed {
		state.Reason = "Current stage gate is blocked."
		if err := s.store.RecordBlocked(ctx, projectID, state); err != nil {
			return State{}, err
		}
		_ = s.store.SaveState(ctx, state)
		return state, nil
	}
	next, ok := nextStage(state.Stage)
	if !ok {
		return State{}, fmt.Errorf("%w: stage %s is not handled by the adaptive workflow", domain.ErrInvalid, state.Stage)
	}
	if _, err := s.projects.ChangeStage(ctx, projectID, expectedRevision, application.StageTransition{
		Next: next, Actor: "workflow", GateChecks: transitionChecks(state.Checks), Unresolved: state.Blockers, Reason: "Stage gate passed.", ApprovalReference: firstApprovalReference(state.ApprovalReferences),
	}); err != nil {
		return State{}, err
	}
	return s.Get(ctx, projectID)
}

func (s *Service) Reopen(ctx context.Context, projectID string, expectedRevision int64, target domain.ProjectStage, reason string) (State, error) {
	if strings.TrimSpace(reason) == "" {
		return State{}, fmt.Errorf("%w: reopen reason is required", domain.ErrInvalid)
	}
	if !earlierStage(target, domain.StageReview) {
		return State{}, fmt.Errorf("%w: only stages before REVIEW can be reopened", domain.ErrInvalid)
	}
	if _, err := s.projects.ChangeStage(ctx, projectID, expectedRevision, application.StageTransition{
		Next: target, Actor: "workflow", Reopened: true, Reason: strings.TrimSpace(reason),
	}); err != nil {
		return State{}, err
	}
	state, err := s.Get(ctx, projectID)
	if err == nil {
		state.Reason = reason
		_ = s.store.SaveState(ctx, state)
	}
	return state, err
}

func transitionChecks(checks []GateCheck) []application.StageGateCheck {
	result := make([]application.StageGateCheck, len(checks))
	for index, check := range checks {
		result[index] = application.StageGateCheck{Code: check.Code, Passed: check.Passed, Message: check.Message}
	}
	return result
}

func firstApprovalReference(references []string) string {
	if len(references) == 0 {
		return ""
	}
	return references[0]
}

func assess(snapshot domain.Snapshot, now time.Time) Assessment {
	project := snapshot.Project
	request := strings.ToLower(project.RawRequest)
	for _, entity := range snapshot.Entities {
		request += " " + strings.ToLower(entity.Title) + " " + strings.ToLower(string(entity.Body))
	}
	systemTypes := []string{"application"}
	concerns := make([]string, 0)
	if containsAny(request, "web", "website", "user interface", "dashboard") {
		systemTypes, concerns = []string{"web_application"}, append(concerns, "interaction")
	}
	if containsAny(request, "api", "service") {
		concerns = appendUnique(concerns, "api")
	}
	if containsAny(request, "data", "store", "stored", "database", "record", "persist") {
		concerns = appendUnique(concerns, "data")
	}
	if containsAny(request, "auth", "login", "secret", "password", "token", "payment", "security") {
		concerns = appendUnique(concerns, "security")
	}
	if containsAny(request, "personal", "privacy", "pii", "profile") {
		concerns = appendUnique(concerns, "privacy")
	}
	if containsAny(request, "failure", "recovery", "reliable", "availability", "durable") {
		concerns = appendUnique(concerns, "reliability")
	}
	if containsAny(request, "latency", "throughput", "performance", "scale") {
		concerns = appendUnique(concerns, "performance")
	}
	if containsAny(request, "concurrent", "asynchronous", "worker", "runtime") {
		concerns = appendUnique(concerns, "runtime")
	}
	if containsAny(request, "deploy", "infrastructure") {
		concerns = appendUnique(concerns, "deployment")
	}
	if containsAny(request, "compatibility", "backward compatible", "public api", "library", "sdk", "protocol", "file format") {
		concerns = appendUnique(concerns, "compatibility")
	}
	if project.Mode == domain.ModeFeature || project.Mode == domain.ModeRefactor {
		concerns = appendUnique(concerns, "repository_change")
		if containsAny(request, "data", "store", "database", "api", "behavior", "auth") {
			concerns = appendUnique(concerns, "migration")
		}
	}
	if project.Mode == domain.ModeSpike {
		concerns = appendUnique(concerns, "experiment")
	}
	criticality := "low"
	if containsAny(request, "health", "payment", "safety", "legal", "security") {
		criticality = "high"
	}
	slices.Sort(concerns)
	return Assessment{ProjectID: project.ID, Mode: project.Mode, SystemTypes: systemTypes, Criticality: criticality, Novelty: 3, DomainUncertainty: 4, TechnicalUncertainty: 3, ChangeScope: 3, DataSensitivity: scoreConcern(concerns, "data"), OperationalExposure: 2, ActiveConcerns: concerns, ProjectRevision: project.Revision, UpdatedAt: now}
}

func evaluate(snapshot domain.Snapshot, assessment Assessment, approvals []domain.Approval, now time.Time) State {
	checks := make([]GateCheck, 0)
	switch snapshot.Project.Stage {
	case domain.StageIntake:
		checks = append(checks,
			GateCheck{Code: "classification_recorded", Passed: assessment.Mode != "", Message: "Project mode and initial assessment are recorded."},
			GateCheck{Code: "request_evidence", Passed: hasEntity(snapshot, domain.KindEvidence, domain.EntityConfirmed, func(entity domain.Entity) bool { return entity.Origin == domain.OriginUser }), Message: "Initial user request is preserved as evidence."},
		)
	case domain.StageFraming:
		checks = append(checks,
			GateCheck{Code: "confirmed_goal", Passed: hasEntity(snapshot, domain.KindGoal, domain.EntityConfirmed, nil), Message: "At least one goal is confirmed."},
			GateCheck{Code: "system_boundary", Passed: hasEntity(snapshot, domain.KindContext, domain.EntityConfirmed, nil), Message: "The system boundary is confirmed."},
			GateCheck{Code: "non_goals", Passed: hasEntity(snapshot, domain.KindScopeItem, domain.EntityConfirmed, bodyDisposition("out_of_scope")), Message: "Non-goals are explicit."},
		)
	case domain.StageContext:
		checks = append(checks,
			GateCheck{Code: "context_confirmed", Passed: hasEntity(snapshot, domain.KindContext, domain.EntityConfirmed, nil), Message: "Baseline and dependencies are confirmed."},
			GateCheck{Code: "constraints_or_gaps", Passed: hasEntity(snapshot, domain.KindConstraint, "", nil) || hasEntity(snapshot, domain.KindAssumption, "", nil) || hasEntity(snapshot, domain.KindQuestion, "", nil), Message: "Material constraints or evidence gaps are visible."},
			GateCheck{Code: "no_blocking_question", Passed: !hasOpenBlockingQuestion(snapshot), Message: "No blocking question remains unanswered."},
		)
	case domain.StageScenarios:
		checks = append(checks,
			GateCheck{Code: "priority_scenarios", Passed: hasEntity(snapshot, domain.KindScenario, domain.EntityConfirmed, nil), Message: "At least one priority scenario is confirmed."},
			GateCheck{Code: "goal_scenario_coverage", Passed: allEntitiesLinked(snapshot, domain.KindGoal, domain.EntityConfirmed, domain.RelationSatisfies, domain.KindScenario, true), Message: "Every confirmed goal is covered by a confirmed scenario."},
		)
	case domain.StageRequirements:
		checks = append(checks,
			GateCheck{Code: "confirmed_requirements", Passed: hasEntity(snapshot, domain.KindRequirement, domain.EntityConfirmed, nil), Message: "At least one requirement is confirmed."},
			GateCheck{Code: "requirement_traceability", Passed: allRequirementsTraced(snapshot), Message: "Confirmed requirements trace to goals or scenarios."},
			GateCheck{Code: "requirements_verifiable", Passed: allRequirementsVerifiable(snapshot), Message: "Confirmed requirements have acceptance conditions."},
			GateCheck{Code: "quality_concerns_addressed", Passed: qualityConcernsAddressed(snapshot, assessment), Message: "Active quality concerns have a quality scenario or risk treatment."},
		)
	case domain.StageShaping:
		checks = append(checks,
			GateCheck{Code: "system_shape", Passed: hasEntity(snapshot, domain.KindSystemElement, "", currentEntity), Message: "A current system boundary or responsibility is represented."},
			GateCheck{Code: "options_visible", Passed: hasEntity(snapshot, domain.KindOption, "", currentEntity), Message: "At least one solution option is visible."},
			GateCheck{Code: "major_risks_treated", Passed: noUntreatedHighRisk(snapshot), Message: "Major feasibility risks have an explicit treatment."},
		)
	case domain.StageDecisions:
		checks = append(checks,
			GateCheck{Code: "decisions_recorded", Passed: hasEntity(snapshot, domain.KindDecision, "", currentEntity), Message: "At least one decision is recorded."},
			GateCheck{Code: "required_approvals", Passed: decisionsApproved(snapshot, approvals), Message: "Every significant current decision is approved at its exact revision."},
		)
	case domain.StageDelivery:
		checks = append(checks,
			GateCheck{Code: "delivery_slices", Passed: hasEntity(snapshot, domain.KindWorkSlice, "", currentEntity), Message: "At least one implementation slice is defined."},
			GateCheck{Code: "requirement_slice_coverage", Passed: allEntitiesLinked(snapshot, domain.KindRequirement, domain.EntityConfirmed, domain.RelationImplements, domain.KindWorkSlice, true), Message: "Confirmed requirements are covered by implementation slices."},
			GateCheck{Code: "slice_completion_evidence", Passed: allSlicesComplete(snapshot), Message: "Every slice has completion conditions and verification references."},
		)
	default:
		checks = append(checks, GateCheck{Code: "handled_by_later_milestone", Passed: false, Message: "This stage is handled by a later workflow milestone."})
	}
	passed := true
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.Passed {
			passed = false
			blockers = append(blockers, check.Code)
		}
	}
	approvalReferences := approvedReferences(approvals)
	return State{ProjectID: snapshot.Project.ID, Stage: snapshot.Project.Stage, ProjectRevision: snapshot.Project.Revision, GatePassed: passed, Checks: checks, Blockers: blockers, Concerns: routeConcerns(assessment), RecommendedRoles: recommendedRoles(snapshot.Project.Stage, assessment), ApprovalReferences: approvalReferences, Assessment: assessment, UpdatedAt: now}
}

func nextStage(stage domain.ProjectStage) (domain.ProjectStage, bool) {
	switch stage {
	case domain.StageIntake:
		return domain.StageFraming, true
	case domain.StageFraming:
		return domain.StageContext, true
	case domain.StageContext:
		return domain.StageScenarios, true
	case domain.StageScenarios:
		return domain.StageRequirements, true
	case domain.StageRequirements:
		return domain.StageShaping, true
	case domain.StageShaping:
		return domain.StageDecisions, true
	case domain.StageDecisions:
		return domain.StageDelivery, true
	case domain.StageDelivery:
		return domain.StageReview, true
	default:
		return "", false
	}
}

func earlierStage(target, than domain.ProjectStage) bool {
	order := map[domain.ProjectStage]int{domain.StageIntake: 0, domain.StageFraming: 1, domain.StageContext: 2, domain.StageScenarios: 3, domain.StageRequirements: 4, domain.StageShaping: 5, domain.StageDecisions: 6, domain.StageDelivery: 7, domain.StageReview: 8}
	value, ok := order[target]
	return ok && value < order[than]
}

func hasEntity(snapshot domain.Snapshot, kind domain.EntityKind, status domain.EntityStatus, predicate func(domain.Entity) bool) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind == kind && (status == "" || entity.Status == status) && (predicate == nil || predicate(entity)) {
			return true
		}
	}
	return false
}

func bodyDisposition(want string) func(domain.Entity) bool {
	return func(entity domain.Entity) bool {
		var body map[string]any
		return json.Unmarshal(entity.Body, &body) == nil && body["disposition"] == want
	}
}

func hasOpenBlockingQuestion(snapshot domain.Snapshot) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindQuestion || entity.Status == domain.EntityConfirmed || entity.Status == domain.EntityRejected {
			continue
		}
		var body map[string]any
		if json.Unmarshal(entity.Body, &body) == nil && body["blocking"] == true && body["disposition"] != "deferred" {
			return true
		}
	}
	return false
}

func currentEntity(entity domain.Entity) bool {
	return entity.Freshness == domain.FreshnessCurrent && entity.Status != domain.EntityRejected && entity.Status != domain.EntitySuperseded
}

func allEntitiesLinked(snapshot domain.Snapshot, subjectKind domain.EntityKind, subjectStatus domain.EntityStatus, relationType domain.RelationType, linkedKind domain.EntityKind, incoming bool) bool {
	entities := make(map[string]domain.Entity, len(snapshot.Entities))
	subjects := make([]domain.Entity, 0)
	for _, entity := range snapshot.Entities {
		entities[entity.ID] = entity
		if entity.Kind == subjectKind && (subjectStatus == "" || entity.Status == subjectStatus) && entity.Freshness == domain.FreshnessCurrent {
			subjects = append(subjects, entity)
		}
	}
	if len(subjects) == 0 {
		return false
	}
	for _, subject := range subjects {
		linked := false
		for _, relation := range snapshot.Relations {
			candidateID := relation.ToID
			matches := relation.FromID == subject.ID
			if incoming {
				candidateID, matches = relation.FromID, relation.ToID == subject.ID
			}
			candidate, ok := entities[candidateID]
			if matches && ok && relation.Type == relationType && candidate.Kind == linkedKind && candidate.Status == domain.EntityConfirmed && currentEntity(candidate) {
				linked = true
				break
			}
		}
		if !linked {
			return false
		}
	}
	return true
}

func allRequirementsTraced(snapshot domain.Snapshot) bool {
	requirements := make([]domain.Entity, 0)
	entities := make(map[string]domain.Entity, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		entities[entity.ID] = entity
		if entity.Kind == domain.KindRequirement && entity.Status == domain.EntityConfirmed && entity.Freshness == domain.FreshnessCurrent {
			requirements = append(requirements, entity)
		}
	}
	if len(requirements) == 0 {
		return false
	}
	for _, requirement := range requirements {
		traced := false
		for _, relation := range snapshot.Relations {
			target, ok := entities[relation.ToID]
			if relation.FromID == requirement.ID && relation.Type == domain.RelationSatisfies && ok && (target.Kind == domain.KindGoal || target.Kind == domain.KindScenario) && currentEntity(target) {
				traced = true
				break
			}
		}
		if !traced {
			return false
		}
	}
	return true
}

func allRequirementsVerifiable(snapshot domain.Snapshot) bool {
	found := false
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindRequirement || entity.Status != domain.EntityConfirmed || entity.Freshness != domain.FreshnessCurrent {
			continue
		}
		found = true
		var body struct {
			AcceptanceConditions []any `json:"acceptance_conditions"`
		}
		if json.Unmarshal(entity.Body, &body) != nil || len(body.AcceptanceConditions) == 0 {
			return false
		}
	}
	return found
}

func qualityConcernsAddressed(snapshot domain.Snapshot, assessment Assessment) bool {
	qualityConcern := false
	for _, concern := range assessment.ActiveConcerns {
		switch concern {
		case "security", "privacy", "reliability", "performance", "runtime", "deployment", "data", "migration", "compatibility":
			qualityConcern = true
		}
	}
	if !qualityConcern {
		return true
	}
	return hasEntity(snapshot, domain.KindQualityScenario, "", currentEntity) || hasEntity(snapshot, domain.KindRisk, "", currentEntity)
}

func noUntreatedHighRisk(snapshot domain.Snapshot) bool {
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindRisk || !currentEntity(entity) {
			continue
		}
		var body map[string]any
		if json.Unmarshal(entity.Body, &body) != nil {
			return false
		}
		severity := strings.ToLower(fmt.Sprint(body["severity"]))
		residual := strings.ToLower(fmt.Sprint(body["residual_risk"]))
		if (severity == "high" || severity == "critical" || severity == "5") && strings.TrimSpace(fmt.Sprint(body["mitigation"])) == "" {
			return false
		}
		if residual == "high" || residual == "critical" || residual == "5" {
			return false
		}
	}
	return true
}

func decisionRequiresApproval(entity domain.Entity) bool {
	var body struct {
		Significance     string `json:"significance"`
		ApprovalRequired bool   `json:"approval_required"`
	}
	return json.Unmarshal(entity.Body, &body) == nil && (body.ApprovalRequired || body.Significance == "architectural")
}

func decisionsApproved(snapshot domain.Snapshot, approvals []domain.Approval) bool {
	found := false
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindDecision || !currentEntity(entity) {
			continue
		}
		found = true
		if !decisionRequiresApproval(entity) {
			continue
		}
		approved := false
		for _, approval := range approvals {
			if approval.SubjectID == entity.ID && approval.SubjectRevision == entity.Revision && approval.Status == domain.ApprovalApproved {
				approved = true
				break
			}
		}
		if !approved {
			return false
		}
	}
	return found
}

func approvedReferences(approvals []domain.Approval) []string {
	references := make([]string, 0)
	for _, approval := range approvals {
		if approval.Status == domain.ApprovalApproved {
			references = append(references, approval.ID)
		}
	}
	slices.Sort(references)
	return references
}

func allSlicesComplete(snapshot domain.Snapshot) bool {
	found := false
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindWorkSlice || !currentEntity(entity) {
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

var concernNames = map[string]struct{}{
	"interaction": {}, "api": {}, "data": {}, "migration": {}, "runtime": {}, "deployment": {}, "security": {}, "privacy": {},
	"reliability": {}, "performance": {}, "compatibility": {}, "repository_change": {}, "experiment": {},
}

func normalizeConcerns(input []string) ([]string, error) {
	result := make([]string, 0, len(input))
	for _, concern := range input {
		concern = strings.TrimSpace(strings.ToLower(concern))
		if _, ok := concernNames[concern]; !ok {
			return nil, fmt.Errorf("%w: unsupported concern %q", domain.ErrInvalid, concern)
		}
		result = appendUnique(result, concern)
	}
	slices.Sort(result)
	return result, nil
}

func routeConcerns(assessment Assessment) []RoutedConcern {
	result := make([]RoutedConcern, 0, len(assessment.ActiveConcerns))
	for _, concern := range assessment.ActiveConcerns {
		result = append(result, RoutedConcern{Name: concern, Mandatory: concern == "security" || concern == "privacy" || concern == "migration" || concern == "compatibility", Triggers: []string{"assessment"}})
	}
	return result
}

func recommendedRoles(stage domain.ProjectStage, assessment Assessment) []domain.AgentRole {
	roles := make([]domain.AgentRole, 0, 2)
	switch stage {
	case domain.StageIntake, domain.StageFraming, domain.StageContext:
		roles = append(roles, domain.RoleDiscovery)
	case domain.StageScenarios, domain.StageRequirements:
		roles = append(roles, domain.RoleRequirements)
	case domain.StageShaping, domain.StageDecisions:
		roles = append(roles, domain.RoleArchitecture)
	case domain.StageDelivery:
		roles = append(roles, domain.RoleDelivery)
	}
	for _, concern := range assessment.ActiveConcerns {
		switch concern {
		case "security", "privacy", "reliability", "performance", "runtime", "deployment", "migration", "compatibility":
			roles = appendUniqueRole(roles, domain.RoleQualityRisk)
		}
	}
	return roles
}

func appendUniqueRole(items []domain.AgentRole, item domain.AgentRole) []domain.AgentRole {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}
func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}
func scoreConcern(items []string, concern string) int {
	for _, item := range items {
		if item == concern {
			return 4
		}
	}
	return 1
}
