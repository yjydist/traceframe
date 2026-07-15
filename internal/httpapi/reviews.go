package httpapi

import (
	"net/http"

	"github.com/yjydist/traceframe/internal/review"
)

func (a *api) registerReviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/reviews", a.listReviewFindings)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/reviews/{findingID}/resolve", a.resolveReviewFinding)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/readiness", a.getReadiness)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/baselines", a.listBaselines)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/baseline", a.createBaseline)
}

func (a *api) listReviewFindings(w http.ResponseWriter, r *http.Request) {
	findings, err := a.reviews.ListFindings(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": findings})
}

func (a *api) resolveReviewFinding(w http.ResponseWriter, r *http.Request) {
	var resolution review.Resolution
	if !decodeJSON(w, r, &resolution) {
		return
	}
	finding, err := a.reviews.ResolveFinding(r.Context(), r.PathValue("projectID"), r.PathValue("findingID"), resolution)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, finding)
}

func (a *api) getReadiness(w http.ResponseWriter, r *http.Request) {
	readiness, err := a.reviews.Readiness(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, readiness)
}

func (a *api) listBaselines(w http.ResponseWriter, r *http.Request) {
	baselines, err := a.reviews.ListBaselines(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"baselines": baselines})
}

func (a *api) createBaseline(w http.ResponseWriter, r *http.Request) {
	var request review.BaselineRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	baseline, err := a.reviews.CreateBaseline(r.Context(), r.PathValue("projectID"), request)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, baseline)
}
