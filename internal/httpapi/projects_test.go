package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/artifacts"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	"github.com/yjydist/traceframe/internal/orchestrator"
	"github.com/yjydist/traceframe/internal/review"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

func TestProjectModelAPIWorkflow(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()

	create := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{
		"name":"Assignment planner",
		"raw_request":"Help students track assignments",
		"mode":"greenfield",
		"output_language":"en"
	}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var project domain.Project
	decodeResponse(t, create, &project)
	if project.ID == "" || project.Revision != 1 || project.Stage != domain.StageIntake {
		t.Fatalf("created project = %#v", project)
	}

	commands := `{
		"expected_revision":1,
		"commands":[
			{"type":"create_entity","entity":{"id":"goal_api","kind":"goal","title":"Submit on time","body":{"outcome":"Students submit assignments on time","success_signals":["Fewer late submissions"],"priority":"must"},"status":"confirmed","origin":"user","confidence":1}},
			{"type":"create_entity","entity":{"id":"scn_api","kind":"scenario","title":"Record assignment","body":{"actor":"student","trigger":"Receives an assignment","preconditions":[],"main_flow":["Records assignment"],"alternative_flows":[],"failure_flows":[],"postconditions":["Assignment is visible"],"importance":"high"},"status":"confirmed","origin":"user","confidence":1}},
			{"type":"create_relation","relation":{"id":"rel_api","from_id":"scn_api","type":"satisfies","to_id":"goal_api","rationale":"Recording work supports timely submission"}}
		]
	}`
	commandResponse := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", commands)
	if commandResponse.Code != http.StatusOK {
		t.Fatalf("commands status = %d, body = %s", commandResponse.Code, commandResponse.Body.String())
	}
	var snapshot domain.Snapshot
	decodeResponse(t, commandResponse, &snapshot)
	if snapshot.Project.Revision != 2 || len(snapshot.Entities) != 3 || len(snapshot.Relations) != 1 {
		t.Fatalf("snapshot after commands = %#v", snapshot)
	}

	invalidCommands := `{
		"expected_revision":2,
		"commands":[
			{"type":"create_entity","entity":{"id":"goal_rolled_back","kind":"goal","title":"Temporary","body":{"outcome":"Temporary","success_signals":[],"priority":"could"}}},
			{"type":"create_relation","relation":{"from_id":"goal_rolled_back","type":"verifies","to_id":"missing","rationale":"invalid"}}
		]
	}`
	invalid := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", invalidCommands)
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid command status = %d, body = %s", invalid.Code, invalid.Body.String())
	}

	snapshotResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/snapshot", "")
	decodeResponse(t, snapshotResponse, &snapshot)
	if snapshot.Project.Revision != 2 || len(snapshot.Entities) != 3 {
		t.Fatalf("invalid command set was not rolled back: %#v", snapshot)
	}

	update := performJSON(t, handler, http.MethodPatch, "/api/v1/projects/"+project.ID, `{"expected_revision":2,"name":"Student planner"}`)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", update.Code, update.Body.String())
	}
	decodeResponse(t, update, &project)
	if project.Name != "Student planner" || project.Revision != 3 {
		t.Fatalf("updated project = %#v", project)
	}

	conflict := performJSON(t, handler, http.MethodPatch, "/api/v1/projects/"+project.ID, `{"expected_revision":2,"name":"Stale update"}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, body = %s", conflict.Code, conflict.Body.String())
	}
	var conflictProblem problem
	decodeResponse(t, conflict, &conflictProblem)
	if conflictProblem.Code != "revision_conflict" || conflictProblem.RequestID == "" || conflict.Header().Get("X-Request-ID") != conflictProblem.RequestID {
		t.Fatalf("conflict problem = %#v, header = %q", conflictProblem, conflict.Header().Get("X-Request-ID"))
	}

	traceResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/traceability", "")
	var trace application.Traceability
	decodeResponse(t, traceResponse, &trace)
	if trace.ProjectRevision != 3 || len(trace.Nodes) != 3 || len(trace.Edges) != 1 || len(trace.Unlinked) != 1 {
		t.Fatalf("traceability = %#v", trace)
	}

	exportOne := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/export?format=json", "")
	exportTwo := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/export?format=json", "")
	if exportOne.Code != http.StatusOK || !bytes.Equal(exportOne.Body.Bytes(), exportTwo.Body.Bytes()) {
		t.Fatalf("exports are not reproducible: status %d/%d", exportOne.Code, exportTwo.Code)
	}

	archive := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/archive", `{"expected_revision":3}`)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive status = %d, body = %s", archive.Code, archive.Body.String())
	}
	list := performJSON(t, handler, http.MethodGet, "/api/v1/projects", "")
	var listed struct {
		Projects []domain.Project `json:"projects"`
	}
	decodeResponse(t, list, &listed)
	if len(listed.Projects) != 0 {
		t.Fatalf("default list includes archived project: %#v", listed.Projects)
	}
	listArchived := performJSON(t, handler, http.MethodGet, "/api/v1/projects?include_archived=true", "")
	decodeResponse(t, listArchived, &listed)
	if len(listed.Projects) != 1 || listed.Projects[0].Status != domain.ProjectArchived {
		t.Fatalf("archived list = %#v", listed.Projects)
	}

	wrongConfirmation := performJSON(t, handler, http.MethodDelete, "/api/v1/projects/"+project.ID, `{"expected_revision":4,"confirm_project_id":"wrong"}`)
	if wrongConfirmation.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation status = %d", wrongConfirmation.Code)
	}
	deleted := performJSON(t, handler, http.MethodDelete, "/api/v1/projects/"+project.ID, `{"expected_revision":4,"confirm_project_id":"`+project.ID+`"}`)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	missing := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted project status = %d, body = %s", missing.Code, missing.Body.String())
	}
	for _, table := range []string{"projects", "project_revisions", "entities", "entity_versions", "relations", "events"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s after deletion: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after permanent deletion = %d, want 0", table, count)
		}
	}
}

func TestProjectAPIStrictJSON(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()

	response := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Test","raw_request":"Test request","unknown":true}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result problem
	decodeResponse(t, response, &result)
	if result.Code != "invalid_json" {
		t.Fatalf("problem = %#v", result)
	}
}

func newProjectTestHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "traceframe.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>Traceframe</title>"), 0o600); err != nil {
		db.Close()
		t.Fatalf("write frontend: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	projects := application.NewProjectService(sqlite.NewRepository(db))
	runtimeStore := sqlite.NewRuntimeRepository(db)
	runs := orchestrator.NewService(projects, runtimeStore, runtimeStore, models.UnconfiguredClient{}, logger)
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	reviewService := review.NewService(projects, sqlite.NewReviewRepository(db))
	artifactService := artifacts.NewService(projects, workflowService, sqlite.NewArtifactRepository(db))
	reviewService.SetArtifactReadiness(artifactService)
	runs.SetApprovalRequester(workflowService)
	runs.SetReviewSubmitter(reviewService)
	return New(db, projects, runs, workflowService, reviewService, artifactService, webDir, logger), db
}

func performJSON(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}
