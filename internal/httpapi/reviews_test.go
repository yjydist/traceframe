package httpapi

import (
	"net/http"
	"testing"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/review"
)

func TestReviewReadinessAndBaselineRoutes(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()
	created := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Review API","raw_request":"Review a bounded project","mode":"greenfield"}`)
	var project domain.Project
	decodeResponse(t, created, &project)

	readinessResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/readiness", "")
	var readiness review.Readiness
	decodeResponse(t, readinessResponse, &readiness)
	if readinessResponse.Code != http.StatusOK || readiness.Ready || len(readiness.Blockers) == 0 {
		t.Fatalf("readiness response = %d, %#v", readinessResponse.Code, readiness)
	}
	reviews := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/reviews", "")
	if reviews.Code != http.StatusOK || reviews.Body.String() != "{\"findings\":[]}\n" {
		t.Fatalf("reviews response = %d, %s", reviews.Code, reviews.Body.String())
	}
	baselines := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/baselines", "")
	if baselines.Code != http.StatusOK || baselines.Body.String() != "{\"baselines\":[]}\n" {
		t.Fatalf("baselines response = %d, %s", baselines.Code, baselines.Body.String())
	}
	blocked := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/baseline", `{"expected_revision":1,"approve":true,"rationale":"Approve"}`)
	if blocked.Code != http.StatusBadRequest {
		t.Fatalf("blocked baseline status = %d, body = %s", blocked.Code, blocked.Body.String())
	}
}
