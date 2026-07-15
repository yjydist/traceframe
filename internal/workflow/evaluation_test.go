package workflow_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

type evaluationFixture struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Mode              domain.ProjectMode `json:"mode"`
	Request           string             `json:"request"`
	ExpectedConcerns  []string           `json:"expected_concerns"`
	ForbiddenConcerns []string           `json:"forbidden_concerns"`
}

func TestVersionedEvaluationFixturesRouteDifferentlyAndReachReview(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "evals", "v1", "fixtures.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures []evaluationFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	fingerprints := make(map[string]struct{})
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			ctx := context.Background()
			db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "evaluation.db"))
			if err != nil {
				t.Fatalf("open database: %v", err)
			}
			defer db.Close()
			projects := application.NewProjectService(sqlite.NewRepository(db))
			snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: fixture.Name, RawRequest: fixture.Request, Mode: fixture.Mode})
			if err != nil {
				t.Fatalf("create project: %v", err)
			}
			snapshot, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 1, Actor: "evaluation", Commands: coherentEvaluationModel(snapshot.Entities[0].ID)})
			if err != nil {
				t.Fatalf("seed coherent model: %v", err)
			}
			service := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
			state, err := service.Get(ctx, snapshot.Project.ID)
			if err != nil {
				t.Fatalf("get workflow: %v", err)
			}
			for _, concern := range fixture.ExpectedConcerns {
				if !slices.Contains(state.Assessment.ActiveConcerns, concern) {
					t.Fatalf("concerns %v missing %q", state.Assessment.ActiveConcerns, concern)
				}
			}
			for _, concern := range fixture.ForbiddenConcerns {
				if slices.Contains(state.Assessment.ActiveConcerns, concern) {
					t.Fatalf("concerns %v unexpectedly contain %q", state.Assessment.ActiveConcerns, concern)
				}
			}
			fingerprint, _ := json.Marshal(state.Assessment.ActiveConcerns)
			fingerprints[string(fingerprint)] = struct{}{}
			for state.Stage != domain.StageReview {
				if !state.GatePassed {
					t.Fatalf("stage %s blocked by %v", state.Stage, state.Blockers)
				}
				state, err = service.Continue(ctx, snapshot.Project.ID, state.ProjectRevision)
				if err != nil {
					t.Fatalf("continue from %s: %v", state.Stage, err)
				}
			}
			if state.ProjectRevision != 10 {
				t.Fatalf("review revision = %d, want 10", state.ProjectRevision)
			}
		})
	}
	if len(fingerprints) < 5 {
		t.Fatalf("routing produced only %d distinct concern sets for %d fixtures", len(fingerprints), len(fixtures))
	}
}

func TestArchitecturalDecisionApprovalUnblocksExactTransition(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "approval-gate.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	projects := application.NewProjectService(sqlite.NewRepository(db))
	snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: "Approval gate", RawRequest: "Shape an architectural boundary", Mode: domain.ModeGreenfield})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	commands := coherentEvaluationModel(snapshot.Entities[0].ID)
	for index := range commands {
		if commands[index].Entity != nil && commands[index].Entity.ID == "decision_eval" {
			commands[index].Entity.Body = json.RawMessage(`{"question":"How should the first increment be shaped?","selected_option_id":"option_eval","rationale":"It produces early verification evidence","consequences":["Scope remains narrow"],"alternatives_considered":["Big-bang delivery"],"revisit_triggers":["The slice cannot demonstrate value"],"significance":"architectural","approval_required":true}`)
		}
	}
	snapshot, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 1, Actor: "evaluation", Commands: commands})
	if err != nil {
		t.Fatalf("seed coherent model: %v", err)
	}
	service := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	state, err := service.Get(ctx, snapshot.Project.ID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	for state.Stage != domain.StageDecisions {
		state, err = service.Continue(ctx, snapshot.Project.ID, state.ProjectRevision)
		if err != nil {
			t.Fatalf("continue workflow: %v", err)
		}
	}
	if state.GatePassed || !slices.Contains(state.Blockers, "required_approvals") {
		t.Fatalf("decision gate before approval = %#v", state)
	}
	approvals, err := service.ListApprovals(ctx, snapshot.Project.ID)
	if err != nil || len(approvals) != 1 || approvals[0].Status != domain.ApprovalPending {
		t.Fatalf("pending approvals = %#v, %v", approvals, err)
	}
	approval, err := service.ResolveApproval(ctx, snapshot.Project.ID, approvals[0].ID, workflow.ApprovalResolution{ExpectedRevision: state.ProjectRevision, Rationale: "The trade-off is accepted"}, true)
	if err != nil || approval.ProjectRevision != state.ProjectRevision {
		t.Fatalf("ResolveApproval() = %#v, %v", approval, err)
	}
	state, err = service.Continue(ctx, snapshot.Project.ID, state.ProjectRevision)
	if err != nil || state.Stage != domain.StageDelivery {
		t.Fatalf("continue after approval = %#v, %v", state, err)
	}
	events, err := projects.Events(ctx, snapshot.Project.ID, 0, 100)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	foundReference := false
	for _, event := range events {
		if event.Type != "workflow.stage_changed" {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(event.Payload, &payload)
		if payload["next"] == string(domain.StageDelivery) && payload["approval_reference"] == approval.ID {
			foundReference = true
		}
	}
	if !foundReference {
		t.Fatal("DECISIONS to DELIVERY transition omitted approval reference")
	}
}

func coherentEvaluationModel(evidenceID string) []application.Command {
	confirmed, origin, confidence := domain.EntityConfirmed, domain.OriginUser, 1.0
	entity := func(id string, kind domain.EntityKind, title, body string) application.Command {
		return application.Command{Type: "create_entity", Entity: &application.EntityDraft{ID: id, Kind: kind, Title: title, Body: json.RawMessage(body), Status: confirmed, Origin: origin, Confidence: &confidence, SourceRefs: []string{evidenceID}}}
	}
	commands := []application.Command{
		entity("goal_eval", domain.KindGoal, "Deliver the requested outcome", `{"outcome":"Deliver the requested outcome responsibly","success_signals":["Primary workflow succeeds"],"priority":"must"}`),
		entity("context_eval", domain.KindContext, "System boundary", `{"current_state":"A baseline exists","system_boundary":"The requested capability","external_dependencies":[],"baseline_behavior":"The capability is unavailable","project_mode":"greenfield"}`),
		entity("scope_eval", domain.KindScopeItem, "Non-goal", `{"statement":"Unrelated capabilities are excluded","disposition":"out_of_scope","rationale":"Keep the change bounded"}`),
		entity("constraint_eval", domain.KindConstraint, "Bounded delivery", `{"category":"time","statement":"Deliver in bounded increments","hard":false,"rationale":"Reduce irreversible risk"}`),
		entity("scenario_eval", domain.KindScenario, "Complete the primary flow", `{"actor":"user","trigger":"The user starts the primary workflow","preconditions":[],"main_flow":["Complete the requested action"],"alternative_flows":[],"failure_flows":["Show a recoverable error"],"postconditions":["The outcome is visible"],"importance":"high"}`),
		entity("requirement_eval", domain.KindRequirement, "Complete the action", `{"statement":"The system shall complete the primary action","category":"functional","rationale":"Satisfy the goal","acceptance_conditions":["The outcome is observable"],"priority":"must","stability":"stable"}`),
		entity("quality_eval", domain.KindQualityScenario, "Recoverable failure", `{"characteristic":"reliability","source":"user","stimulus":"The action fails","environment":"normal operation","artifact":"primary workflow","response":"The system reports a recoverable error","measure":"No partial state remains"}`),
		entity("risk_eval", domain.KindRisk, "Delivery risk", `{"category":"delivery","cause":"Unknown implementation details","event":"Delivery is delayed","impact":"The outcome arrives late","likelihood":"medium","severity":"medium","mitigation":"Deliver one verified slice","evidence_needed":["Slice verification"],"residual_risk":"low"}`),
		entity("option_eval", domain.KindOption, "Bounded vertical slice", `{"decision_topic":"Delivery shape","description":"Deliver one bounded vertical slice","benefits":["Early evidence"],"costs":["Limited first scope"],"risks":["Follow-up work"],"fit_to_constraints":["Bounded delivery"],"evidence_refs":["evidence"]}`),
		entity("decision_eval", domain.KindDecision, "Select bounded delivery", `{"question":"How should the first increment be shaped?","selected_option_id":"option_eval","rationale":"It produces early verification evidence","consequences":["Scope remains narrow"],"alternatives_considered":["Big-bang delivery"],"revisit_triggers":["The slice cannot demonstrate value"],"significance":"local","approval_required":false}`),
		entity("element_eval", domain.KindSystemElement, "Primary system", `{"element_type":"software_system","responsibilities":["Complete the primary workflow"],"boundary":"Requested capability","lifecycle":"project"}`),
		entity("verification_eval", domain.KindVerification, "Verify primary requirement", `{"target_ref":"requirement_eval","method":"test","procedure":"Exercise the primary workflow","expected_result":"The observable outcome appears","environment":"test","owner":"delivery"}`),
		entity("slice_eval", domain.KindWorkSlice, "Deliver primary workflow", `{"outcome":"A user completes the primary workflow","included":["requirement_eval"],"excluded":[],"dependencies":[],"verification_refs":["verification_eval"],"risk_reduction":"Produces executable evidence","completion_conditions":["Verification passes"],"order_hint":1}`),
	}
	relation := func(id, from string, kind domain.RelationType, to string) application.Command {
		return application.Command{Type: "create_relation", Relation: &application.RelationDraft{ID: id, FromID: from, Type: kind, ToID: to, Rationale: "Evaluation traceability"}}
	}
	return append(commands,
		relation("rel_scenario_goal", "scenario_eval", domain.RelationSatisfies, "goal_eval"),
		relation("rel_requirement_scenario", "requirement_eval", domain.RelationSatisfies, "scenario_eval"),
		relation("rel_verification_requirement", "verification_eval", domain.RelationVerifies, "requirement_eval"),
		relation("rel_decision_option", "decision_eval", domain.RelationSelects, "option_eval"),
		relation("rel_slice_requirement", "slice_eval", domain.RelationImplements, "requirement_eval"),
	)
}
