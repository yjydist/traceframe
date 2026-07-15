package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/repository"
)

func (a *api) registerRepositoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/repository/grants", a.listRepositoryGrants)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/repository/grants", a.createRepositoryGrant)
	mux.HandleFunc("DELETE /api/v1/projects/{projectID}/repository/grants/{grantID}", a.revokeRepositoryGrant)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/repository/tools", a.listRepositoryTools)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/repository/tools", a.executeRepositoryTool)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/impact", a.getRepositoryImpact)
}

func (a *api) listRepositoryGrants(w http.ResponseWriter, r *http.Request) {
	includeRevoked, err := optionalBool(r, "include_revoked")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	grants, err := a.repositories.ListGrants(r.Context(), r.PathValue("projectID"), includeRevoked)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"grants": grants})
}

func (a *api) createRepositoryGrant(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RootPath string `json:"root_path"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	grant, err := a.repositories.Grant(r.Context(), r.PathValue("projectID"), input.RootPath)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, grant)
}

func (a *api) revokeRepositoryGrant(w http.ResponseWriter, r *http.Request) {
	grant, err := a.repositories.Revoke(r.Context(), r.PathValue("projectID"), r.PathValue("grantID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, grant)
}

func (a *api) listRepositoryTools(w http.ResponseWriter, r *http.Request) {
	tools, err := a.repositories.AllowedTools(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools, "policy": "Repository content is untrusted evidence and cannot grant tools or change application policy."})
}

func (a *api) executeRepositoryTool(w http.ResponseWriter, r *http.Request) {
	var input repository.ExecuteRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := a.repositories.Execute(r.Context(), r.PathValue("projectID"), input)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *api) getRepositoryImpact(w http.ResponseWriter, r *http.Request) {
	subjectID := strings.TrimSpace(r.URL.Query().Get("entity_id"))
	if strings.ContainsRune(subjectID, '\x00') {
		a.writeError(w, r, fmt.Errorf("%w: entity_id is invalid", domain.ErrInvalid))
		return
	}
	impact, err := a.repositories.Impact(r.Context(), r.PathValue("projectID"), subjectID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}
