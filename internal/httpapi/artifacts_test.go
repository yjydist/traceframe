package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	artifactmodel "github.com/yjydist/traceframe/internal/artifacts"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/review"
)

func TestArtifactRenderExportAndInvalidationAPI(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()
	created := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Artifact API","raw_request":"Create a bounded local tool","mode":"greenfield"}`)
	var project domain.Project
	decodeResponse(t, created, &project)
	confidence := 1.0
	commands, _ := json.Marshal(application.CommandEnvelope{ExpectedRevision: 1, Commands: []application.Command{
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_artifact_api", Kind: domain.KindGoal, Title: "Stable handoff", Body: json.RawMessage(`{"outcome":"Produce a stable handoff","success_signals":["Export is reproducible"],"priority":"must"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "scope_artifact_api", Kind: domain.KindScopeItem, Title: "No deployment", Body: json.RawMessage(`{"statement":"Deployment is excluded","disposition":"out_of_scope","rationale":"The tool is local"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
	}})
	if response := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", string(commands)); response.Code != http.StatusOK {
		t.Fatalf("seed model status = %d, body = %s", response.Code, response.Body.String())
	}

	rendered := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/artifacts/render", `{"expected_revision":2,"renderers":["html","markdown","json","mermaid"]}`)
	var first artifactmodel.RenderResult
	decodeResponse(t, rendered, &first)
	if rendered.Code != http.StatusCreated || first.SourceRevision != 2 || len(first.Versions) == 0 {
		t.Fatalf("render result = %d, %#v", rendered.Code, first)
	}
	renderers := make(map[artifactmodel.RendererType]bool)
	for _, version := range first.Versions {
		renderers[version.RendererType] = true
	}
	for _, renderer := range []artifactmodel.RendererType{artifactmodel.RendererHTML, artifactmodel.RendererMarkdown, artifactmodel.RendererJSON, artifactmodel.RendererMermaid} {
		if !renderers[renderer] {
			t.Fatalf("renderer %s was not exercised: %v", renderer, renderers)
		}
	}
	repeated := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/artifacts/render", `{"expected_revision":2,"renderers":["html","markdown","json","mermaid"]}`)
	var second artifactmodel.RenderResult
	decodeResponse(t, repeated, &second)
	if len(first.Versions) != len(second.Versions) {
		t.Fatalf("repeat version count = %d/%d", len(first.Versions), len(second.Versions))
	}
	for index := range first.Versions {
		if first.Versions[index].ID != second.Versions[index].ID || first.Versions[index].Checksum != second.Versions[index].Checksum {
			t.Fatalf("repeat render %d changed version identity", index)
		}
	}

	listedResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/artifacts", "")
	var listed struct {
		Artifacts []artifactmodel.Artifact `json:"artifacts"`
	}
	decodeResponse(t, listedResponse, &listed)
	var packet *artifactmodel.Artifact
	for index := range listed.Artifacts {
		if listed.Artifacts[index].ViewType == "implementation_packet" && listed.Artifacts[index].RendererType == artifactmodel.RendererMarkdown {
			packet = &listed.Artifacts[index]
		}
	}
	if packet == nil || packet.Latest == nil || !strings.Contains(packet.Latest.Content, "entity:goal_artifact_api") {
		t.Fatalf("implementation packet = %#v", packet)
	}
	raw := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/artifacts/"+packet.ID+"?raw=true", "")
	if raw.Code != http.StatusOK || !strings.HasPrefix(raw.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("raw artifact = %d, %q", raw.Code, raw.Header().Get("Content-Type"))
	}
	exported := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/export?format=markdown", "")
	if exported.Code != http.StatusOK || !strings.Contains(exported.Body.String(), "entity:goal_artifact_api") {
		t.Fatalf("markdown export = %d, %s", exported.Code, exported.Body.String())
	}
	readinessResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/readiness", "")
	var readiness review.Readiness
	decodeResponse(t, readinessResponse, &readiness)
	if !checkPassed(readiness, "mandatory_artifacts") {
		t.Fatalf("mandatory artifact readiness = %#v", readiness)
	}

	updated, _ := json.Marshal(application.CommandEnvelope{ExpectedRevision: 2, Commands: []application.Command{{Type: "update_entity", EntityID: "goal_artifact_api", ExpectedEntityRevision: 1, Changes: &application.EntityChanges{Body: json.RawMessage(`{"outcome":"Produce a revised handoff","success_signals":["Export is reproducible"],"priority":"must"}`)}}}})
	if response := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", string(updated)); response.Code != http.StatusOK {
		t.Fatalf("update model status = %d, body = %s", response.Code, response.Body.String())
	}
	listedResponse = performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/artifacts", "")
	decodeResponse(t, listedResponse, &listed)
	for _, artifact := range listed.Artifacts {
		if artifact.Latest == nil || !artifact.Latest.Stale {
			t.Fatalf("artifact remained current after model change: %#v", artifact)
		}
	}
}

func checkPassed(readiness review.Readiness, code string) bool {
	for _, check := range readiness.Checks {
		if check.Code == code {
			return check.Passed
		}
	}
	return false
}
