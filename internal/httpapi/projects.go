package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

const maxJSONBody = 1 << 20

func (a *api) registerProjectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/projects", a.createProject)
	mux.HandleFunc("GET /api/v1/projects", a.listProjects)
	mux.HandleFunc("GET /api/v1/projects/{projectID}", a.getProject)
	mux.HandleFunc("PATCH /api/v1/projects/{projectID}", a.updateProject)
	mux.HandleFunc("DELETE /api/v1/projects/{projectID}", a.deleteProject)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/archive", a.archiveProject)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/snapshot", a.getSnapshot)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/revisions", a.listRevisions)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/entities", a.listEntities)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/entities/{entityID}", a.getEntity)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/commands", a.applyCommands)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/relations", a.listRelations)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/traceability", a.getTraceability)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/export", a.exportProject)
}

func (a *api) createProject(w http.ResponseWriter, r *http.Request) {
	var input application.CreateProjectInput
	if !decodeJSON(w, r, &input) {
		return
	}
	snapshot, err := a.projects.Create(r.Context(), input)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, snapshot.Project)
}

func (a *api) listProjects(w http.ResponseWriter, r *http.Request) {
	includeArchived, err := optionalBool(r, "include_archived")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	projects, err := a.projects.List(r.Context(), includeArchived)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (a *api) getProject(w http.ResponseWriter, r *http.Request) {
	project, err := a.projects.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (a *api) updateProject(w http.ResponseWriter, r *http.Request) {
	var input application.UpdateProjectInput
	if !decodeJSON(w, r, &input) {
		return
	}
	snapshot, err := a.projects.Update(r.Context(), r.PathValue("projectID"), input)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot.Project)
}

type revisionRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type deleteRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	ConfirmProjectID string `json:"confirm_project_id"`
}

func (a *api) archiveProject(w http.ResponseWriter, r *http.Request) {
	var input revisionRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	snapshot, err := a.projects.Archive(r.Context(), r.PathValue("projectID"), input.ExpectedRevision)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot.Project)
}

func (a *api) deleteProject(w http.ResponseWriter, r *http.Request) {
	var input deleteRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	projectID := r.PathValue("projectID")
	if input.ConfirmProjectID != projectID {
		a.writeError(w, r, fmt.Errorf("%w: confirm_project_id must exactly match the project id", domain.ErrInvalid))
		return
	}
	if err := a.projects.Delete(r.Context(), projectID, input.ExpectedRevision); err != nil {
		a.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) getSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.projects.Snapshot(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (a *api) listRevisions(w http.ResponseWriter, r *http.Request) {
	revisions, err := a.projects.Revisions(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
}

func (a *api) listEntities(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.projects.Snapshot(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	entities := make([]domain.Entity, 0, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		if value := r.URL.Query().Get("kind"); value != "" && string(entity.Kind) != value {
			continue
		}
		if value := r.URL.Query().Get("status"); value != "" && string(entity.Status) != value {
			continue
		}
		if value := r.URL.Query().Get("origin"); value != "" && string(entity.Origin) != value {
			continue
		}
		if value := r.URL.Query().Get("freshness"); value != "" && string(entity.Freshness) != value {
			continue
		}
		entities = append(entities, entity)
	}
	writeJSON(w, http.StatusOK, map[string]any{"project_revision": snapshot.Project.Revision, "entities": entities})
}

func (a *api) getEntity(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.projects.Snapshot(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	for _, entity := range snapshot.Entities {
		if entity.ID == r.PathValue("entityID") {
			writeJSON(w, http.StatusOK, entity)
			return
		}
	}
	a.writeError(w, r, fmt.Errorf("%w: entity %s", application.ErrNotFound, r.PathValue("entityID")))
}

func (a *api) applyCommands(w http.ResponseWriter, r *http.Request) {
	var envelope application.CommandEnvelope
	if !decodeJSON(w, r, &envelope) {
		return
	}
	snapshot, err := a.projects.ApplyCommands(r.Context(), r.PathValue("projectID"), envelope)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (a *api) listRelations(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.projects.Snapshot(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project_revision": snapshot.Project.Revision, "relations": snapshot.Relations})
}

func (a *api) getTraceability(w http.ResponseWriter, r *http.Request) {
	traceability, err := a.projects.Traceability(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, traceability)
}

func (a *api) exportProject(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != "" && format != "json" {
		a.writeError(w, r, fmt.Errorf("%w: format must be json", domain.ErrInvalid))
		return
	}
	data, err := a.projects.ExportJSON(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="traceframe-project.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type problem struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	RequestID string         `json:"request_id"`
}

func (a *api) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "The request could not be completed."
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code, message = http.StatusBadRequest, "invalid_argument", err.Error()
	case errors.Is(err, application.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, application.ErrConflict):
		status, code, message = http.StatusConflict, "revision_conflict", err.Error()
	default:
		a.logger.Error("request failed", "request_id", requestIDFromContext(r.Context()), "error", err)
	}
	writeJSON(w, status, problem{Code: code, Message: message, Details: map[string]any{}, RequestID: requestIDFromContext(r.Context())})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, problem{Code: "invalid_json", Message: err.Error(), Details: map[string]any{}, RequestID: requestIDFromContext(r.Context())})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, problem{Code: "invalid_json", Message: "request body must contain one JSON value", Details: map[string]any{}, RequestID: requestIDFromContext(r.Context())})
		return false
	}
	return true
}

func optionalBool(r *http.Request, key string) (bool, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%w: %s must be a boolean", domain.ErrInvalid, key)
	}
	return parsed, nil
}
