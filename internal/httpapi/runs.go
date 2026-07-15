package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/orchestrator"
)

func (a *api) registerRunRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/projects/{projectID}/runs", a.createRun)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/runs", a.listRuns)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/runs/{runID}", a.getRun)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/runs/{runID}/cancel", a.cancelRun)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/events", a.streamEvents)
}

func (a *api) createRun(w http.ResponseWriter, r *http.Request) {
	var request orchestrator.RunRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if headerKey := strings.TrimSpace(r.Header.Get("Idempotency-Key")); headerKey != "" {
		if request.IdempotencyKey != "" && request.IdempotencyKey != headerKey {
			a.writeError(w, r, fmt.Errorf("%w: body and header idempotency keys differ", domain.ErrInvalid))
			return
		}
		request.IdempotencyKey = headerKey
	}
	run, created, err := a.runs.CreateRun(r.Context(), r.PathValue("projectID"), request)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, run)
}

func (a *api) listRuns(w http.ResponseWriter, r *http.Request) {
	limit, err := optionalInt(r, "limit")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	runs, err := a.runs.ListRuns(r.Context(), r.PathValue("projectID"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (a *api) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := a.runs.GetRun(r.Context(), r.PathValue("projectID"), r.PathValue("runID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *api) cancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := a.runs.Cancel(r.Context(), r.PathValue("projectID"), r.PathValue("runID"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

type eventEnvelope struct {
	ProjectID      string          `json:"project_id"`
	OccurredAt     time.Time       `json:"occurred_at"`
	PayloadVersion int             `json:"payload_version"`
	Payload        json.RawMessage `json:"payload"`
}

func (a *api) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		a.writeError(w, r, fmt.Errorf("event streaming is unsupported"))
		return
	}
	after, err := lastEventID(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if _, err := a.projects.Snapshot(r.Context(), r.PathValue("projectID")); err != nil {
		a.writeError(w, r, err)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(200 * time.Millisecond)
	heartbeat := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer heartbeat.Stop()
	for {
		events, err := a.projects.Events(r.Context(), r.PathValue("projectID"), after, 100)
		if err != nil {
			return
		}
		for _, event := range events {
			data, _ := json.Marshal(eventEnvelope{ProjectID: event.ProjectID, OccurredAt: event.OccurredAt, PayloadVersion: 1, Payload: event.Payload})
			_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, data)
			after = event.Sequence
		}
		if len(events) > 0 {
			flusher.Flush()
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func lastEventID(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%w: Last-Event-ID must be a non-negative integer", domain.ErrInvalid)
	}
	return parsed, nil
}

func optionalInt(r *http.Request, key string) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%w: %s must be a non-negative integer", domain.ErrInvalid, key)
	}
	return parsed, nil
}
