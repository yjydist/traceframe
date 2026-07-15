package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/artifacts"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	"github.com/yjydist/traceframe/internal/orchestrator"
	"github.com/yjydist/traceframe/internal/repository"
	"github.com/yjydist/traceframe/internal/review"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

func TestRepositoryAPIGrantEvidenceImpactAndRevocation(t *testing.T) {
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "traceframe.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	projects := application.NewProjectService(sqlite.NewRepository(db))
	runtimeStore := sqlite.NewRuntimeRepository(db)
	runs := orchestrator.NewService(projects, runtimeStore, runtimeStore, models.UnconfiguredClient{}, logger)
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	reviewService := review.NewService(projects, sqlite.NewReviewRepository(db))
	artifactService := artifacts.NewService(projects, workflowService, sqlite.NewArtifactRepository(db))
	repositoryService := repository.NewService(projects, sqlite.NewRepositoryAccessStore(db), repository.DefaultOptions())
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>Traceframe</title>"), 0o600); err != nil {
		t.Fatalf("write frontend: %v", err)
	}
	handler := NewWithRepository(db, projects, runs, workflowService, reviewService, artifactService, repositoryService, webDir, logger)

	create := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Auth feature","raw_request":"Add passkeys","mode":"feature"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", create.Code, create.Body.String())
	}
	var project domain.Project
	decodeResponse(t, create, &project)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "auth.go"), []byte("package auth\n\nfunc VerifyPasskey() bool { return true }\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	grantBody, _ := json.Marshal(map[string]string{"root_path": root})
	grantResponse := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/repository/grants", string(grantBody))
	if grantResponse.Code != http.StatusCreated {
		t.Fatalf("grant: %d %s", grantResponse.Code, grantResponse.Body.String())
	}
	var grant repository.Grant
	decodeResponse(t, grantResponse, &grant)

	toolBody, _ := json.Marshal(repository.ExecuteRequest{GrantID: grant.ID, Tool: repository.ToolReadFile, Path: "auth.go", StartLine: 3, EndLine: 3, RecordEvidence: true, ExpectedRevision: 1})
	toolResponse := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/repository/tools", string(toolBody))
	if toolResponse.Code != http.StatusOK {
		t.Fatalf("tool: %d %s", toolResponse.Code, toolResponse.Body.String())
	}
	var result repository.ToolResult
	decodeResponse(t, toolResponse, &result)
	if len(result.EvidenceIDs) != 1 || result.ModelRevision != 2 || result.Entries[0].SHA256 == "" || result.Entries[0].StartLine != 3 {
		t.Fatalf("tool result = %#v", result)
	}

	impactResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/impact", "")
	if impactResponse.Code != http.StatusOK {
		t.Fatalf("impact: %d %s", impactResponse.Code, impactResponse.Body.String())
	}
	var impact repository.ImpactAnalysis
	decodeResponse(t, impactResponse, &impact)
	if len(impact.RepositoryEvidenceIDs) != 1 || impact.RepositoryEvidenceIDs[0] != result.EvidenceIDs[0] {
		t.Fatalf("impact = %#v", impact)
	}

	revoke := performJSON(t, handler, http.MethodDelete, "/api/v1/projects/"+project.ID+"/repository/grants/"+grant.ID, "")
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", revoke.Code, revoke.Body.String())
	}
	tools := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/repository/tools", "")
	var allowed struct {
		Tools []string `json:"tools"`
	}
	decodeResponse(t, tools, &allowed)
	if len(allowed.Tools) != 0 {
		t.Fatalf("tools after revoke = %v", allowed.Tools)
	}

	var auditCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repository_tool_calls WHERE project_id = ? AND status = 'completed'`, project.ID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("repository tool audit count = %d, err = %v", auditCount, err)
	}
}
