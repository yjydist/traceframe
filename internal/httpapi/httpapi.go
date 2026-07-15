package httpapi

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	artifactmodel "github.com/yjydist/traceframe/internal/artifacts"
	"github.com/yjydist/traceframe/internal/orchestrator"
	"github.com/yjydist/traceframe/internal/review"
	"github.com/yjydist/traceframe/internal/workflow"
)

type api struct {
	db        *sql.DB
	projects  *application.ProjectService
	runs      *orchestrator.Service
	workflow  *workflow.Service
	reviews   *review.Service
	artifacts *artifactmodel.Service
	logger    *slog.Logger
}

func New(db *sql.DB, projects *application.ProjectService, runs *orchestrator.Service, workflowService *workflow.Service, reviewService *review.Service, artifactService *artifactmodel.Service, webDir string, logger *slog.Logger) http.Handler {
	service := &api{db: db, projects: projects, runs: runs, workflow: workflowService, reviews: reviewService, artifacts: artifactService, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", service.health)
	service.registerProjectRoutes(mux)
	service.registerRunRoutes(mux)
	service.registerWorkflowRoutes(mux)
	service.registerReviewRoutes(mux)
	service.registerArtifactRoutes(mux)
	mux.Handle("/", spaHandler(webDir, logger))
	return requestID(requestLogger(mux, logger))
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()

	if err := a.db.PingContext(ctx); err != nil {
		a.logger.Error("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":   "unavailable",
			"database": "unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"database": "ok",
	})
}

func spaHandler(webDir string, logger *slog.Logger) http.Handler {
	root := os.DirFS(webDir)
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requestedPath == "." || requestedPath == "" {
			requestedPath = "index.html"
		}

		if info, err := fs.Stat(root, requestedPath); err == nil && !info.IsDir() {
			if requestedPath == "index.html" {
				w.Header().Set("Cache-Control", "no-store")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		if _, err := fs.Stat(root, "index.html"); err != nil {
			logger.Warn("frontend build is unavailable", "web_dir", webDir)
			http.Error(w, "frontend build is unavailable", http.StatusServiceUnavailable)
			return
		}

		clone := r.Clone(r.Context())
		clone.URL.Path = "/"
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, clone)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
