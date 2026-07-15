package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

func TestRunAPIIdempotencyCancellationAndSSEReplay(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()
	server := httptest.NewServer(handler)
	defer server.Close()

	createResponse := performRemoteJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/projects", `{"name":"Run project","raw_request":"Frame an assignment planner","mode":"greenfield"}`, "")
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", createResponse.StatusCode)
	}
	var project domain.Project
	decodeRemote(t, createResponse, &project)

	runBody := `{"role":"discovery","task":"Identify the outcome, boundary, and highest-value unknown"}`
	queuedResponse := performRemoteJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/projects/"+project.ID+"/runs", runBody, "run-api-1")
	if queuedResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("queue status = %d", queuedResponse.StatusCode)
	}
	var run domain.AgentRun
	decodeRemote(t, queuedResponse, &run)
	if run.State != domain.RunQueued || run.BaseRevision != 1 {
		t.Fatalf("queued run = %#v", run)
	}

	idempotentResponse := performRemoteJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/projects/"+project.ID+"/runs", runBody, "run-api-1")
	if idempotentResponse.StatusCode != http.StatusOK {
		t.Fatalf("idempotent status = %d", idempotentResponse.StatusCode)
	}
	var existing domain.AgentRun
	decodeRemote(t, idempotentResponse, &existing)
	if existing.ID != run.ID {
		t.Fatalf("idempotent run id = %s, want %s", existing.ID, run.ID)
	}

	conflictResponse := performRemoteJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/projects/"+project.ID+"/runs", `{"role":"discovery","task":"A different task"}`, "run-api-1")
	if conflictResponse.StatusCode != http.StatusConflict {
		t.Fatalf("idempotency conflict status = %d", conflictResponse.StatusCode)
	}
	conflictResponse.Body.Close()

	eventRequest, cancelEvents := context.WithCancel(context.Background())
	request, _ := http.NewRequestWithContext(eventRequest, http.MethodGet, server.URL+"/api/v1/projects/"+project.ID+"/events", nil)
	request.Header.Set("Last-Event-ID", "1")
	eventResponse, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("open SSE stream: %v", err)
	}
	if eventResponse.StatusCode != http.StatusOK || eventResponse.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SSE response = %d %q", eventResponse.StatusCode, eventResponse.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(eventResponse.Body)
	lines := make([]string, 0, 3)
	deadline := time.After(500 * time.Millisecond)
	for len(lines) < 3 {
		lineChannel := make(chan string, 1)
		go func() {
			if scanner.Scan() {
				lineChannel <- scanner.Text()
			}
		}()
		select {
		case line := <-lineChannel:
			if line != "" {
				lines = append(lines, line)
			}
		case <-deadline:
			t.Fatal("SSE first state event exceeded 500ms")
		}
	}
	if lines[0] != "id: 2" || lines[1] != "event: run.queued" || !strings.HasPrefix(lines[2], "data: ") {
		t.Fatalf("SSE replay lines = %#v", lines)
	}
	cancelEvents()
	eventResponse.Body.Close()

	cancelResponse := performRemoteJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/projects/"+project.ID+"/runs/"+run.ID+"/cancel", "", "")
	if cancelResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("cancel status = %d", cancelResponse.StatusCode)
	}
	var cancelled domain.AgentRun
	decodeRemote(t, cancelResponse, &cancelled)
	if cancelled.State != domain.RunCancelled || cancelled.CancelRequestedAt == nil {
		t.Fatalf("cancelled run = %#v", cancelled)
	}

	listResponse := performRemoteJSON(t, server.Client(), http.MethodGet, server.URL+"/api/v1/projects/"+project.ID+"/runs", "", "")
	var list struct {
		Runs []domain.AgentRun `json:"runs"`
	}
	decodeRemote(t, listResponse, &list)
	if len(list.Runs) != 1 || list.Runs[0].State != domain.RunCancelled {
		t.Fatalf("run list = %#v", list.Runs)
	}
}

func performRemoteJSON(t *testing.T, client *http.Client, method, target, body, idempotencyKey string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request %s: %v", target, err)
	}
	return response
}

func decodeRemote(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
