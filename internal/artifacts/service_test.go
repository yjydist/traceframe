package artifacts_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	artifactmodel "github.com/yjydist/traceframe/internal/artifacts"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/review"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

func TestBaselineRenderingIsReproducibleAndModelChangesInvalidateVersions(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "artifacts.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	projects := application.NewProjectService(sqlite.NewRepository(db))
	snapshot, err := projects.Create(ctx, application.CreateProjectInput{Name: "Artifact project", RawRequest: "Create a bounded local tool", Mode: domain.ModeGreenfield})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	confidence := 1.0
	snapshot, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 1, Commands: []application.Command{
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_artifact", Kind: domain.KindGoal, Title: "Stable outcome", Body: json.RawMessage(`{"outcome":"Produce a stable artifact","success_signals":["Packet is reproducible"],"priority":"must"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "scope_artifact", Kind: domain.KindScopeItem, Title: "Explicit non-goal", Body: json.RawMessage(`{"statement":"Network deployment is excluded","disposition":"out_of_scope","rationale":"The tool is local"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
	}})
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	store := sqlite.NewArtifactRepository(db)
	service := artifactmodel.NewService(projects, workflowService, store)
	approvedAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	baseline, created, err := sqlite.NewReviewRepository(db).CreateBaseline(ctx, review.Baseline{ID: "baseline_artifact", ProjectID: snapshot.Project.ID, ProjectRevision: 2, RoutedConcerns: []string{}, ApprovalActor: "user", ApprovalRationale: "Freeze reproducible source", ApprovedAt: approvedAt, CreatedAt: approvedAt}, 2)
	if err != nil || !created {
		t.Fatalf("create baseline = %#v, %v, %v", baseline, created, err)
	}

	first, err := service.Render(ctx, snapshot.Project.ID, artifactmodel.RenderRequest{BaselineID: baseline.ID, Renderers: []artifactmodel.RendererType{artifactmodel.RendererMarkdown}})
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := service.Render(ctx, snapshot.Project.ID, artifactmodel.RenderRequest{BaselineID: baseline.ID, Renderers: []artifactmodel.RendererType{artifactmodel.RendererMarkdown}})
	if err != nil || len(first.Versions) != len(second.Versions) {
		t.Fatalf("second render = %#v, %v", second, err)
	}
	packet := ""
	for index := range first.Versions {
		if first.Versions[index].ID != second.Versions[index].ID || first.Versions[index].Checksum != second.Versions[index].Checksum || first.Versions[index].Content != second.Versions[index].Content {
			t.Fatalf("render %d was not idempotent", index)
		}
		artifact, _ := service.Get(ctx, snapshot.Project.ID, first.Versions[index].ArtifactID)
		if artifact.ViewType == "implementation_packet" {
			packet = first.Versions[index].Content
		}
	}
	if packet == "" || !strings.Contains(packet, "entity:goal_artifact") || !strings.Contains(packet, "baseline_artifact") {
		t.Fatalf("implementation packet missing stable metadata:\n%s", packet)
	}
	exported, err := service.ExportMarkdown(ctx, snapshot.Project.ID)
	if err != nil || exported != packet {
		t.Fatalf("markdown export mismatch: %v", err)
	}
	current, missing, err := service.CurrentMandatoryViews(ctx, snapshot.Project.ID, 2)
	if err != nil || !current || len(missing) != 0 {
		t.Fatalf("mandatory views = %v, %v, %v", current, missing, err)
	}

	updatedBody := json.RawMessage(`{"outcome":"Produce a revised stable artifact","success_signals":["Packet is reproducible"],"priority":"must"}`)
	_, err = projects.ApplyCommands(ctx, snapshot.Project.ID, application.CommandEnvelope{ExpectedRevision: 2, Commands: []application.Command{{Type: "update_entity", EntityID: "goal_artifact", ExpectedEntityRevision: 1, Changes: &application.EntityChanges{Body: updatedBody}}}})
	if err != nil {
		t.Fatalf("update model: %v", err)
	}
	listed, err := service.List(ctx, snapshot.Project.ID)
	if err != nil || len(listed) == 0 {
		t.Fatalf("list artifacts = %#v, %v", listed, err)
	}
	for _, artifact := range listed {
		if artifact.Latest == nil || !artifact.Latest.Stale {
			t.Fatalf("artifact was not invalidated: %#v", artifact)
		}
	}
	current, missing, err = service.CurrentMandatoryViews(ctx, snapshot.Project.ID, 3)
	if err != nil || current || len(missing) == 0 {
		t.Fatalf("post-change mandatory views = %v, %v, %v", current, missing, err)
	}
	afterChange, err := service.Render(ctx, snapshot.Project.ID, artifactmodel.RenderRequest{BaselineID: baseline.ID, Renderers: []artifactmodel.RendererType{artifactmodel.RendererMarkdown}})
	if err != nil {
		t.Fatalf("rerender baseline after change: %v", err)
	}
	for index := range first.Versions {
		if first.Versions[index].Checksum != afterChange.Versions[index].Checksum || first.Versions[index].Content != afterChange.Versions[index].Content {
			t.Fatalf("baseline render changed after current model mutation")
		}
	}
	var invalidations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE project_id = ? AND type = 'artifact.invalidated'`, snapshot.Project.ID).Scan(&invalidations); err != nil || invalidations != len(listed) {
		t.Fatalf("artifact invalidations = %d, want %d, error %v", invalidations, len(listed), err)
	}
}
