package httpapi

import (
	"net/http"

	"github.com/yjydist/traceframe/internal/domain"
)

func (a *api) registerWorkflowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/workflow", a.getWorkflow)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/workflow/continue", a.continueWorkflow)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/workflow/reopen", a.reopenWorkflow)
}

func (a *api) getWorkflow(w http.ResponseWriter, r *http.Request) {
	state, err := a.workflow.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *api) continueWorkflow(w http.ResponseWriter, r *http.Request) {
	var request revisionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := a.workflow.Continue(r.Context(), r.PathValue("projectID"), request.ExpectedRevision)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type reopenRequest struct {
	ExpectedRevision int64               `json:"expected_revision"`
	Stage            domain.ProjectStage `json:"stage"`
	Reason           string              `json:"reason"`
}

func (a *api) reopenWorkflow(w http.ResponseWriter, r *http.Request) {
	var request reopenRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := a.workflow.Reopen(r.Context(), r.PathValue("projectID"), request.ExpectedRevision, request.Stage, request.Reason)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
