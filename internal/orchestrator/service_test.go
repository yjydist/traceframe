package orchestrator

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/yjydist/traceframe/internal/agents"
	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	"github.com/yjydist/traceframe/internal/models/fake"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
)

func TestDiscoveryRunRepairsSchemaAndAppliesProposal(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "orchestrator.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	projectStore := sqlite.NewRepository(db)
	projects := application.NewProjectService(projectStore)
	snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: "Assignment planner", RawRequest: "Help students track assignments", Mode: domain.ModeGreenfield})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(snapshot.Entities) != 1 || snapshot.Entities[0].Kind != domain.KindEvidence {
		t.Fatalf("initial user evidence missing: %#v", snapshot.Entities)
	}

	runtimeStore := sqlite.NewRuntimeRepository(db)
	model := fake.New()
	service := NewService(projects, runtimeStore, runtimeStore, model, slog.New(slog.NewTextHandler(io.Discard, nil)))
	budget := domain.DefaultRunBudget()
	budget.MaxModelTurns = 3
	run, created, err := service.CreateRun(ctx, snapshot.Project.ID, RunRequest{Role: domain.RoleDiscovery, Task: "Identify the initial outcome, boundary, non-goals, and highest-value question", IdempotencyKey: "discovery-1", Budget: &budget})
	if err != nil || !created {
		t.Fatalf("CreateRun() = %#v, %v, %v", run, created, err)
	}
	evidenceID := snapshot.Entities[0].ID
	proposal := agents.Proposal{
		RunID: run.ID, BaseRevision: run.BaseRevision, Summary: "Established an initial framing model",
		Commands: []application.Command{
			{Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_discovery", Kind: domain.KindGoal, Title: "Submit work on time", Body: json.RawMessage(`{"outcome":"Students submit assignments on time","success_signals":["Fewer late submissions"],"priority":"must"}`), SourceRefs: []string{evidenceID}}},
			{Type: "create_entity", Entity: &application.EntityDraft{ID: "scope_discovery", Kind: domain.KindScopeItem, Title: "No automated grading", Body: json.RawMessage(`{"statement":"Automated grading is excluded","disposition":"out_of_scope","rationale":"The request only covers tracking"}`), SourceRefs: []string{evidenceID}}},
			{Type: "create_entity", Entity: &application.EntityDraft{ID: "context_discovery", Kind: domain.KindContext, Title: "Initial boundary", Body: json.RawMessage(`{"current_state":"Assignments are tracked inconsistently","system_boundary":"Assignment planning and reminders","external_dependencies":[],"baseline_behavior":"Students use ad hoc notes","project_mode":"greenfield"}`), SourceRefs: []string{evidenceID}}},
			{Type: "create_entity", Entity: &application.EntityDraft{ID: "constraint_discovery", Kind: domain.KindConstraint, Title: "Student-managed tracking", Body: json.RawMessage(`{"category":"organizational","statement":"Students maintain their own assignment records","hard":false,"rationale":"The request names students as the users tracking assignments"}`), SourceRefs: []string{evidenceID}}},
			{Type: "create_entity", Entity: &application.EntityDraft{ID: "assumption_discovery", Kind: domain.KindAssumption, Title: "Assignments have due dates", Body: json.RawMessage(`{"statement":"Tracked assignments have due dates","impact_if_false":"medium","validation_method":"Confirm with the project owner","owner":"user"}`), SourceRefs: []string{evidenceID}}},
			{Type: "create_entity", Entity: &application.EntityDraft{ID: "qst_discovery", Kind: domain.KindQuestion, Title: "Primary user", Body: json.RawMessage(`{"prompt":"Who is the primary student audience?","reason":"The answer changes workflow and accessibility needs","answer_type":"text","impact":4,"uncertainty":4,"irreversibility":2,"blocking":false}`)}},
		},
		Warnings: []string{}, Unresolved: []string{"Primary audience is unknown"}, RecommendedNextAction: "ask_prioritized_questions",
	}
	proposalJSON, _ := json.Marshal(proposal)
	model.Push(
		fake.Result{Err: &models.ProviderError{Code: "rate_limit", Message: "retry", Transient: true}},
		fake.Result{Response: models.GenerateResponse{Output: json.RawMessage(`{"invalid":true}`), ModelIdentifier: "fake-discovery", Usage: models.Usage{InputTokens: 100, OutputTokens: 10}}},
		fake.Result{Response: models.GenerateResponse{Output: proposalJSON, ModelIdentifier: "fake-discovery", ProviderRequestID: "provider_1", Usage: models.Usage{InputTokens: 120, OutputTokens: 80}}},
	)
	processed, err := service.RunOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("RunOnce() = %v, %v", processed, err)
	}
	completed, err := service.GetRun(ctx, snapshot.Project.ID, run.ID)
	if err != nil || completed.State != domain.RunCompleted || completed.Usage.ModelTurns != 3 || completed.ProposalChecksum == "" || completed.ApplicationOutcome != "applied" {
		t.Fatalf("completed run = %#v, %v", completed, err)
	}
	updated, err := projects.Snapshot(ctx, snapshot.Project.ID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if updated.Project.Revision != 2 || len(updated.Entities) != 7 {
		t.Fatalf("updated snapshot = revision %d, entities %d", updated.Project.Revision, len(updated.Entities))
	}
	for _, entity := range updated.Entities {
		if entity.Origin == domain.OriginAgent && (entity.Status != domain.EntityProposed || entity.Freshness != domain.FreshnessCurrent) {
			t.Fatalf("agent entity escaped proposal policy: %#v", entity)
		}
	}
	if len(model.Requests()) != 3 {
		t.Fatalf("model request count = %d, want schema repair", len(model.Requests()))
	}
	var stepCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agent_run_steps WHERE run_id = ?`, run.ID).Scan(&stepCount); err != nil || stepCount != 5 {
		t.Fatalf("run step count = %d, %v", stepCount, err)
	}
}

type blockingModel struct{ entered chan struct{} }

func (m *blockingModel) Generate(ctx context.Context, _ models.GenerateRequest) (models.GenerateResponse, error) {
	close(m.entered)
	<-ctx.Done()
	return models.GenerateResponse{}, ctx.Err()
}

func (m *blockingModel) Stream(context.Context, models.GenerateRequest) (<-chan models.StreamEvent, error) {
	return nil, nil
}

func TestActiveRunCancellationStopsProposalApplication(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	projects := application.NewProjectService(sqlite.NewRepository(db))
	snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: "Cancellation", RawRequest: "Test cancellation", Mode: domain.ModeSpike})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	runtimeStore := sqlite.NewRuntimeRepository(db)
	model := &blockingModel{entered: make(chan struct{})}
	service := NewService(projects, runtimeStore, runtimeStore, model, slog.New(slog.NewTextHandler(io.Discard, nil)))
	run, _, err := service.CreateRun(ctx, snapshot.Project.ID, RunRequest{Task: "Identify the spike decision criteria", IdempotencyKey: "cancel-1"})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := service.RunOnce(ctx); done <- err }()
	select {
	case <-model.entered:
	case <-time.After(time.Second):
		t.Fatal("model was not called")
	}
	if _, err := service.Cancel(ctx, snapshot.Project.ID, run.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunOnce() after cancellation error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cooperative cancellation exceeded two seconds")
	}
	cancelled, err := service.GetRun(ctx, snapshot.Project.ID, run.ID)
	if err != nil || cancelled.State != domain.RunCancelled {
		t.Fatalf("cancelled run = %#v, %v", cancelled, err)
	}
	current, _ := projects.Snapshot(ctx, snapshot.Project.ID)
	if current.Project.Revision != 1 || len(current.Entities) != 1 {
		t.Fatalf("cancelled run mutated project: %#v", current)
	}
}

func TestConcurrentProposalRequiresReconciliation(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "reconciliation.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	projects := application.NewProjectService(sqlite.NewRepository(db))
	snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: "Concurrent", RawRequest: "Frame one bounded outcome", Mode: domain.ModeGreenfield})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	runtimeStore := sqlite.NewRuntimeRepository(db)
	model := fake.New()
	service := NewService(projects, runtimeStore, runtimeStore, model, slog.New(slog.NewTextHandler(io.Discard, nil)))
	first, _, err := service.CreateRun(ctx, snapshot.Project.ID, RunRequest{Role: domain.RoleDiscovery, Task: "Propose the first evidence-backed goal", IdempotencyKey: "concurrent-1"})
	if err != nil {
		t.Fatalf("CreateRun(first) error = %v", err)
	}
	second, _, err := service.CreateRun(ctx, snapshot.Project.ID, RunRequest{Role: domain.RoleDiscovery, Task: "Propose an independent evidence-backed goal", IdempotencyKey: "concurrent-2"})
	if err != nil {
		t.Fatalf("CreateRun(second) error = %v", err)
	}
	proposal := agents.Proposal{RunID: first.ID, BaseRevision: 1, Summary: "First proposal", Commands: []application.Command{{Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_concurrent", Kind: domain.KindGoal, Title: "Bounded outcome", Body: json.RawMessage(`{"outcome":"Produce one bounded outcome","success_signals":["Outcome is visible"],"priority":"must"}`), SourceRefs: []string{snapshot.Entities[0].ID}}}}, Warnings: []string{}, Unresolved: []string{}, RecommendedNextAction: "confirm_goal"}
	output, _ := json.Marshal(proposal)
	model.Push(fake.Result{Response: models.GenerateResponse{Output: output, ModelIdentifier: "fake"}})
	if processed, err := service.RunOnce(ctx); err != nil || !processed {
		t.Fatalf("RunOnce(first) = %v, %v", processed, err)
	}
	if processed, err := service.RunOnce(ctx); err == nil || !processed {
		t.Fatalf("RunOnce(second) = %v, %v, want reconciliation error", processed, err)
	}
	conflicted, err := service.GetRun(ctx, snapshot.Project.ID, second.ID)
	if err != nil || conflicted.State != domain.RunFailed || conflicted.ErrorCode != "reconciliation_required" || conflicted.ApplicationOutcome != "reconciliation_required" {
		t.Fatalf("conflicted run = %#v, %v", conflicted, err)
	}
	if len(model.Requests()) != 1 {
		t.Fatalf("model request count = %d, want stale run stopped before model", len(model.Requests()))
	}
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE project_id = ? AND type = 'run.reconciliation_required'`, snapshot.Project.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("reconciliation event count = %d, %v", events, err)
	}
}
