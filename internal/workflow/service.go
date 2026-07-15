package workflow

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

func (s *Service) Get(ctx context.Context, projectID string) (State, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	assessment := assess(snapshot.Project, s.now().UTC())
	if err := s.store.SaveAssessment(ctx, assessment); err != nil {
		return State{}, err
	}
	state := evaluate(snapshot, assessment, s.now().UTC())
	if err := s.store.SaveState(ctx, state); err != nil {
		return State{}, err
	}
	return state, nil
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
		return State{}, fmt.Errorf("%w: stage %s is not handled by the framing workflow", domain.ErrInvalid, state.Stage)
	}
	if _, err := s.projects.ChangeStage(ctx, projectID, expectedRevision, application.StageTransition{
		Next: next, Actor: "workflow", GateChecks: transitionChecks(state.Checks), Unresolved: state.Blockers, Reason: "Stage gate passed.",
	}); err != nil {
		return State{}, err
	}
	return s.Get(ctx, projectID)
}

func (s *Service) Reopen(ctx context.Context, projectID string, expectedRevision int64, target domain.ProjectStage, reason string) (State, error) {
	if strings.TrimSpace(reason) == "" {
		return State{}, fmt.Errorf("%w: reopen reason is required", domain.ErrInvalid)
	}
	if !earlierStage(target, domain.StageScenarios) {
		return State{}, fmt.Errorf("%w: only INTAKE, FRAMING, or CONTEXT can be reopened in this milestone", domain.ErrInvalid)
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

func assess(project domain.Project, now time.Time) Assessment {
	request := strings.ToLower(project.RawRequest)
	systemTypes := []string{"application"}
	concerns := make([]string, 0)
	if containsAny(request, "web", "website", "ui", "dashboard") {
		systemTypes, concerns = []string{"web_application"}, append(concerns, "interaction")
	}
	if containsAny(request, "api", "service") {
		concerns = appendUnique(concerns, "api")
	}
	if containsAny(request, "data", "store", "database", "record") {
		concerns = appendUnique(concerns, "data")
	}
	if containsAny(request, "auth", "login", "secret", "payment") {
		concerns = appendUnique(concerns, "security")
	}
	if project.Mode == domain.ModeFeature || project.Mode == domain.ModeRefactor {
		concerns = appendUnique(concerns, "repository_change")
	}
	if project.Mode == domain.ModeSpike {
		concerns = appendUnique(concerns, "experiment")
	}
	criticality := "low"
	if containsAny(request, "health", "payment", "safety", "legal", "security") {
		criticality = "high"
	}
	return Assessment{ProjectID: project.ID, Mode: project.Mode, SystemTypes: systemTypes, Criticality: criticality, Novelty: 3, DomainUncertainty: 4, TechnicalUncertainty: 3, ChangeScope: 3, DataSensitivity: scoreConcern(concerns, "data"), OperationalExposure: 2, ActiveConcerns: concerns, ProjectRevision: project.Revision, UpdatedAt: now}
}

func evaluate(snapshot domain.Snapshot, assessment Assessment, now time.Time) State {
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
	return State{ProjectID: snapshot.Project.ID, Stage: snapshot.Project.Stage, ProjectRevision: snapshot.Project.Revision, GatePassed: passed, Checks: checks, Blockers: blockers, Assessment: assessment, UpdatedAt: now}
}

func nextStage(stage domain.ProjectStage) (domain.ProjectStage, bool) {
	switch stage {
	case domain.StageIntake:
		return domain.StageFraming, true
	case domain.StageFraming:
		return domain.StageContext, true
	case domain.StageContext:
		return domain.StageScenarios, true
	default:
		return "", false
	}
}

func earlierStage(target, than domain.ProjectStage) bool {
	order := map[domain.ProjectStage]int{domain.StageIntake: 0, domain.StageFraming: 1, domain.StageContext: 2, domain.StageScenarios: 3}
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
