package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/models"
	"github.com/yjydist/traceframe/internal/orchestrator"
	"github.com/yjydist/traceframe/internal/storage/sqlite"
	"github.com/yjydist/traceframe/internal/workflow"
)

func TestHealthAndSPA(t *testing.T) {
	db, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	webDir := t.TempDir()
	index := "<!doctype html><title>Traceframe</title><main>workspace</main>"
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte(index), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}

	projects := application.NewProjectService(sqlite.NewRepository(db))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtimeStore := sqlite.NewRuntimeRepository(db)
	runs := orchestrator.NewService(projects, runtimeStore, runtimeStore, models.UnconfiguredClient{}, logger)
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	handler := New(db, projects, runs, workflowService, webDir, logger)

	t.Run("health", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if !strings.Contains(response.Body.String(), `"status":"ok"`) {
			t.Fatalf("body = %q, want ok status", response.Body.String())
		}
	})

	t.Run("client route falls back to index", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/projects/example/overview", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if response.Body.String() != index {
			t.Fatalf("body = %q, want index document", response.Body.String())
		}
	})
}

func TestMissingFrontendReturnsServiceUnavailable(t *testing.T) {
	db, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	projects := application.NewProjectService(sqlite.NewRepository(db))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtimeStore := sqlite.NewRuntimeRepository(db)
	runs := orchestrator.NewService(projects, runtimeStore, runtimeStore, models.UnconfiguredClient{}, logger)
	workflowService := workflow.NewService(projects, sqlite.NewWorkflowRepository(db))
	handler := New(db, projects, runs, workflowService, t.TempDir(), logger)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
