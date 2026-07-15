package review_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yjydist/traceframe/internal/agents"
	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	"github.com/yjydist/traceframe/internal/models/fake"
	"github.com/yjydist/traceframe/internal/orchestrator"
	"github.com/yjydist/traceframe/internal/review"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

func TestReviewBlockersAndImmutableExactRevisionBaseline(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	projects := application.NewProjectService(sqlite.NewRepository(db))
	snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: "Readiness", RawRequest: "Review a consequential design", Mode: domain.ModeGreenfield})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	evidenceID := snapshot.Entities[0].ID
	snapshot, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 1, Actor: "test", Commands: readinessModel(evidenceID)})
	if err != nil {
		t.Fatalf("seed readiness model: %v", err)
	}
	snapshot, err = projects.ChangeStage(ctx, snapshot.Project.ID, 2, application.StageTransition{Next: domain.StageReview, Actor: "test", Reason: "Seed review state"})
	if err != nil {
		t.Fatalf("enter review: %v", err)
	}
	reviewService := review.NewService(projects, sqlite.NewReviewRepository(db))
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	readiness, err := reviewService.Readiness(ctx, snapshot.Project.ID)
	if err != nil {
		t.Fatalf("Readiness() error = %v", err)
	}
	for _, blocker := range []string{"architectural_approvals", "residual_risk_acceptance", "active_conflicts"} {
		if !slices.Contains(readiness.Blockers, blocker) {
			t.Fatalf("initial blockers %v missing %s", readiness.Blockers, blocker)
		}
	}
	if _, err := reviewService.CreateBaseline(ctx, snapshot.Project.ID, review.BaselineRequest{ExpectedRevision: 3, Approve: true, Rationale: "Premature"}); err == nil {
		t.Fatal("baseline created with readiness blockers")
	}

	approvals, err := workflowService.ListApprovals(ctx, snapshot.Project.ID)
	if err != nil || len(approvals) != 2 {
		t.Fatalf("required approvals = %#v, %v", approvals, err)
	}
	for _, approval := range approvals {
		resolved, err := workflowService.ResolveApproval(ctx, snapshot.Project.ID, approval.ID, workflow.ApprovalResolution{ExpectedRevision: 3, Rationale: "Exact subject trade-off accepted"}, true)
		if err != nil || resolved.ProjectRevision != 3 {
			t.Fatalf("resolve approval = %#v, %v", resolved, err)
		}
	}
	snapshot, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 3, Actor: "user", Commands: []application.Command{{Type: "delete_relation", RelationID: "conflict_readiness"}}})
	if err != nil {
		t.Fatalf("resolve contradiction: %v", err)
	}
	if snapshot.Project.Revision != 4 {
		t.Fatalf("revision after resolving conflict = %d", snapshot.Project.Revision)
	}

	runtimeStore := sqlite.NewRuntimeRepository(db)
	model := fake.New()
	runs := orchestrator.NewService(projects, runtimeStore, runtimeStore, model, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runs.SetReviewSubmitter(reviewService)
	run, _, err := runs.CreateRun(ctx, snapshot.Project.ID, orchestrator.RunRequest{Role: domain.RoleCritic, Task: "Independently find readiness blockers", IdempotencyKey: "critic-review-1"})
	if err != nil {
		t.Fatalf("create critic run: %v", err)
	}
	proposal := agents.ReviewProposal{RunID: run.ID, BaseRevision: 4, Summary: "Found an unsupported readiness claim", Findings: []review.FindingDraft{{ID: "finding_blocking", Severity: review.SeverityBlocking, Category: "evidence", AffectedEntityIDs: []string{"goal_readiness"}, Claim: "The success claim lacks representative evidence", Evidence: "Only the initial statement supports the success signal", RecommendedResolution: "Add representative validation evidence"}}, Warnings: []string{}, Unresolved: []string{}, RecommendedNextAction: "resolve_blocking_finding"}
	output, _ := json.Marshal(proposal)
	model.Push(fake.Result{Response: models.GenerateResponse{Output: output, ModelIdentifier: "fake-critic"}})
	if processed, err := runs.RunOnce(ctx); err != nil || !processed {
		t.Fatalf("run critic = %v, %v", processed, err)
	}
	current, _ := projects.Snapshot(ctx, snapshot.Project.ID)
	if current.Project.Revision != 4 {
		t.Fatalf("critic mutated project revision to %d", current.Project.Revision)
	}
	readiness, _ = reviewService.Readiness(ctx, snapshot.Project.ID)
	if !slices.Contains(readiness.Blockers, "blocking_findings") {
		t.Fatalf("readiness blockers after critic = %v", readiness.Blockers)
	}
	if _, err := reviewService.ResolveFinding(ctx, snapshot.Project.ID, "finding_blocking", review.Resolution{ExpectedRevision: 4, Status: review.FindingRiskAccepted, Rationale: "Accept"}); err == nil {
		t.Fatal("blocking finding was risk accepted")
	}
	if _, err := reviewService.ResolveFinding(ctx, snapshot.Project.ID, "finding_blocking", review.Resolution{ExpectedRevision: 4, Status: review.FindingDismissed, Rationale: "Not applicable"}); err == nil {
		t.Fatal("blocking finding was dismissed without counter-evidence")
	}
	if _, err := reviewService.ResolveFinding(ctx, snapshot.Project.ID, "finding_blocking", review.Resolution{ExpectedRevision: 4, Status: review.FindingDismissed, Rationale: "Not applicable", CounterEvidenceRefs: []string{"missing_evidence"}}); err == nil {
		t.Fatal("blocking finding was dismissed with nonexistent counter-evidence")
	}
	finding, err := reviewService.ResolveFinding(ctx, snapshot.Project.ID, "finding_blocking", review.Resolution{ExpectedRevision: 4, Status: review.FindingDismissed, Rationale: "The confirmed user evidence establishes the bounded success signal", CounterEvidenceRefs: []string{evidenceID}})
	if err != nil || finding.Status != review.FindingDismissed {
		t.Fatalf("dismiss with counter-evidence = %#v, %v", finding, err)
	}

	readiness, err = reviewService.Readiness(ctx, snapshot.Project.ID)
	if err != nil || !readiness.Ready || len(readiness.Blockers) != 0 {
		t.Fatalf("final readiness = %#v, %v", readiness, err)
	}
	baseline, err := reviewService.CreateBaseline(ctx, snapshot.Project.ID, review.BaselineRequest{ExpectedRevision: 4, Approve: true, Rationale: "Approve this exact reviewed model"})
	if err != nil {
		t.Fatalf("CreateBaseline() error = %v", err)
	}
	if baseline.ProjectRevision != 4 || baseline.Checksum == "" || len(baseline.Snapshot) == 0 {
		t.Fatalf("baseline = %#v", baseline)
	}
	readySnapshot, _ := projects.Snapshot(ctx, snapshot.Project.ID)
	if readySnapshot.Project.Stage != domain.StageReady || readySnapshot.Project.Status != domain.ProjectReady || readySnapshot.Project.Revision != 5 {
		t.Fatalf("ready snapshot = %#v", readySnapshot.Project)
	}
	events, err := projects.Events(ctx, snapshot.Project.ID, 0, 100)
	if err != nil {
		t.Fatalf("list baseline events: %v", err)
	}
	foundBaselineReference := false
	for _, event := range events {
		if event.Type != "workflow.stage_changed" {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(event.Payload, &payload)
		if payload["next"] == string(domain.StageReady) && payload["approval_reference"] == baseline.ID {
			foundBaselineReference = true
		}
	}
	if !foundBaselineReference {
		t.Fatal("READY transition omitted exact baseline approval reference")
	}
	var frozen domain.Snapshot
	if err := json.Unmarshal(baseline.Snapshot, &frozen); err != nil || frozen.Project.Revision != 4 {
		t.Fatalf("frozen baseline snapshot = %#v, %v", frozen.Project, err)
	}

	confidence := 1.0
	_, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 5, Actor: "user", Commands: []application.Command{{Type: "create_entity", Entity: &application.EntityDraft{ID: "assumption_after_baseline", Kind: domain.KindAssumption, Title: "Later assumption", Body: json.RawMessage(`{"statement":"A later change is needed","impact_if_false":"low","validation_method":"Ask the user","owner":"user"}`), Status: domain.EntityProposed, Origin: domain.OriginUser, Confidence: &confidence}}}})
	if err != nil {
		t.Fatalf("change after baseline: %v", err)
	}
	baselines, err := reviewService.ListBaselines(ctx, snapshot.Project.ID)
	if err != nil || len(baselines) != 1 || baselines[0].Checksum != baseline.Checksum || !bytes.Equal(baselines[0].Snapshot, baseline.Snapshot) {
		t.Fatalf("baseline changed after later revision: %#v, %v", baselines, err)
	}
}

func readinessModel(evidenceID string) []application.Command {
	confirmed, origin, confidence := domain.EntityConfirmed, domain.OriginUser, 1.0
	entity := func(id string, kind domain.EntityKind, title, body string) application.Command {
		return application.Command{Type: "create_entity", Entity: &application.EntityDraft{ID: id, Kind: kind, Title: title, Body: json.RawMessage(body), Status: confirmed, Origin: origin, Confidence: &confidence, SourceRefs: []string{evidenceID}}}
	}
	commands := []application.Command{
		entity("goal_readiness", domain.KindGoal, "Reviewed outcome", `{"outcome":"Deliver a reviewed outcome","success_signals":["Verification passes"],"priority":"must"}`),
		entity("scope_readiness", domain.KindScopeItem, "Explicit non-goal", `{"statement":"Unrelated workflows are excluded","disposition":"out_of_scope","rationale":"Keep the baseline bounded"}`),
		entity("requirement_readiness", domain.KindRequirement, "Verified behavior", `{"statement":"The primary behavior shall succeed","category":"functional","rationale":"Satisfy the goal","acceptance_conditions":["The result is visible"],"priority":"must","stability":"stable"}`),
		entity("verification_readiness", domain.KindVerification, "Verify behavior", `{"target_ref":"requirement_readiness","method":"test","procedure":"Exercise the behavior","expected_result":"The result is visible","environment":"test","owner":"delivery"}`),
		entity("slice_readiness", domain.KindWorkSlice, "Deliver behavior", `{"outcome":"Primary behavior works","included":["requirement_readiness"],"excluded":[],"dependencies":[],"verification_refs":["verification_readiness"],"risk_reduction":"Produces evidence","completion_conditions":["Verification passes"],"order_hint":1}`),
		entity("option_readiness", domain.KindOption, "Isolated boundary", `{"decision_topic":"Boundary","description":"Use an isolated boundary","benefits":["Isolation"],"costs":["Integration"],"risks":["Mismatch"],"fit_to_constraints":[],"evidence_refs":[]}`),
		entity("decision_readiness", domain.KindDecision, "Select boundary", `{"question":"Which boundary?","selected_option_id":"option_readiness","rationale":"Isolation limits change","consequences":["An interface is required"],"alternatives_considered":["Inline behavior"],"revisit_triggers":["Integration cost dominates"],"significance":"architectural","approval_required":true}`),
		entity("risk_readiness", domain.KindRisk, "Accepted residual risk", `{"category":"architecture","cause":"The boundary may be wrong","event":"Integration cost grows","impact":"Delivery slows","likelihood":"medium","severity":"high","mitigation":"Measure integration cost","evidence_needed":["Integration measurement"],"residual_risk":"high"}`),
	}
	relation := func(id, from string, kind domain.RelationType, to string) application.Command {
		return application.Command{Type: "create_relation", Relation: &application.RelationDraft{ID: id, FromID: from, Type: kind, ToID: to, Rationale: "Readiness traceability"}}
	}
	return append(commands,
		relation("rel_requirement_goal", "requirement_readiness", domain.RelationSatisfies, "goal_readiness"),
		relation("rel_verification_requirement", "verification_readiness", domain.RelationVerifies, "requirement_readiness"),
		relation("rel_slice_requirement", "slice_readiness", domain.RelationImplements, "requirement_readiness"),
		relation("rel_decision_option", "decision_readiness", domain.RelationSelects, "option_readiness"),
		relation("conflict_readiness", "goal_readiness", domain.RelationConflictsWith, "scope_readiness"),
	)
}
