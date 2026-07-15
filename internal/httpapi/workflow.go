package httpapi

import (
	"net/http"

	"github.com/yjydist/traceframe/internal/domain"
	workflowmodel "github.com/yjydist/traceframe/internal/workflow"
)

func (a *api) registerWorkflowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/workflow", a.getWorkflow)
	mux.HandleFunc("PUT /api/v1/projects/{projectID}/workflow/assessment", a.correctAssessment)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/workflow/continue", a.continueWorkflow)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/workflow/reopen", a.reopenWorkflow)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/approvals", a.listApprovals)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/approvals/{approvalID}/approve", a.approve)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/approvals/{approvalID}/reject", a.rejectApproval)
}

func (a *api) getWorkflow(w http.ResponseWriter, r *http.Request) {
	state, err := a.workflow.Get(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (a *api) correctAssessment(w http.ResponseWriter, r *http.Request) {
	var correction workflowmodel.AssessmentCorrection
	if !decodeJSON(w, r, &correction) {
		return
	}
	assessment, err := a.workflow.CorrectAssessment(r.Context(), r.PathValue("projectID"), correction)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, assessment)
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

func (a *api) listApprovals(w http.ResponseWriter, r *http.Request) {
	approvals, err := a.workflow.ListApprovals(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
}

func (a *api) approve(w http.ResponseWriter, r *http.Request) {
	a.resolveApproval(w, r, true)
}

func (a *api) rejectApproval(w http.ResponseWriter, r *http.Request) {
	a.resolveApproval(w, r, false)
}

func (a *api) resolveApproval(w http.ResponseWriter, r *http.Request, approve bool) {
	var resolution workflowmodel.ApprovalResolution
	if !decodeJSON(w, r, &resolution) {
		return
	}
	approval, err := a.workflow.ResolveApproval(r.Context(), r.PathValue("projectID"), r.PathValue("approvalID"), resolution, approve)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, approval)
}
