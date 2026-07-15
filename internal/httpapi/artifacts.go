package httpapi

import (
	"net/http"
	"strings"

	artifactmodel "github.com/yjydist/traceframe/internal/artifacts"
)

func (a *api) registerArtifactRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/artifacts", a.listArtifacts)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/artifacts/render", a.renderArtifacts)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/artifacts/{artifactID}", a.getArtifact)
}

func (a *api) listArtifacts(w http.ResponseWriter, r *http.Request) {
	artifacts, err := a.artifacts.List(r.Context(), r.PathValue("projectID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}

func (a *api) renderArtifacts(w http.ResponseWriter, r *http.Request) {
	var request artifactmodel.RenderRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := a.artifacts.Render(r.Context(), r.PathValue("projectID"), request)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (a *api) getArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, err := a.artifacts.Get(r.Context(), r.PathValue("projectID"), r.PathValue("artifactID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if strings.EqualFold(r.URL.Query().Get("raw"), "true") && artifact.Latest != nil {
		w.Header().Set("Content-Type", artifact.Latest.ContentType)
		if artifact.RendererType == artifactmodel.RendererHTML {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'none'")
		}
		w.Header().Set("ETag", `"`+artifact.Latest.Checksum+`"`)
		if artifact.Latest.Stale {
			w.Header().Set("Warning", `299 traceframe "artifact source is stale"`)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(artifact.Latest.Content))
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}
