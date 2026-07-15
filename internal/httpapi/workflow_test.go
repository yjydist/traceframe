package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	workflowmodel "github.com/yjydist/traceframe/internal/workflow"
)

func TestFramingWorkflowGatesAndReopen(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()
	create := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Workflow","raw_request":"Build a web planner that stores assignments","mode":"greenfield"}`)
	var project domain.Project
	decodeResponse(t, create, &project)

	stateResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/workflow", "")
	var state workflowmodel.State
	decodeResponse(t, stateResponse, &state)
	if state.Stage != domain.StageIntake || !state.GatePassed || len(state.Assessment.ActiveConcerns) == 0 {
		t.Fatalf("intake state = %#v", state)
	}
	var assessmentCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM assessments WHERE project_id = ?`, project.ID).Scan(&assessmentCount); err != nil || assessmentCount != 1 {
		t.Fatalf("assessment count = %d, %v", assessmentCount, err)
	}
	corrected := performJSON(t, handler, http.MethodPut, "/api/v1/projects/"+project.ID+"/workflow/assessment", `{"expected_revision":1,"criticality":"high","active_concerns":["security","interaction"]}`)
	var assessment workflowmodel.Assessment
	decodeResponse(t, corrected, &assessment)
	if !assessment.Corrected || assessment.Criticality != "high" || len(assessment.ActiveConcerns) != 2 {
		t.Fatalf("corrected assessment = %#v", assessment)
	}

	framing := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/workflow/continue", `{"expected_revision":1}`)
	decodeResponse(t, framing, &state)
	if state.Stage != domain.StageFraming || state.ProjectRevision != 2 || state.GatePassed {
		t.Fatalf("framing state = %#v", state)
	}
	if !state.Assessment.Corrected || state.Assessment.Criticality != "high" {
		t.Fatalf("assessment correction was not preserved: %#v", state.Assessment)
	}
	blocked := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/workflow/continue", `{"expected_revision":2}`)
	decodeResponse(t, blocked, &state)
	if state.ProjectRevision != 2 || state.GatePassed || state.Reason == "" {
		t.Fatalf("blocked state = %#v", state)
	}
	var blockedEvents int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE project_id = ? AND type = 'workflow.blocked'`, project.ID).Scan(&blockedEvents); err != nil || blockedEvents != 1 {
		t.Fatalf("blocked event count = %d, %v", blockedEvents, err)
	}

	confidence := 0.8
	commands := []application.Command{
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_workflow", Kind: domain.KindGoal, Title: "Track assignments", Body: json.RawMessage(`{"outcome":"Students submit work on time","success_signals":["Fewer late assignments"],"priority":"must"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "context_workflow", Kind: domain.KindContext, Title: "Planner boundary", Body: json.RawMessage(`{"current_state":"Students use notes","system_boundary":"Assignment planning","external_dependencies":[],"baseline_behavior":"Manual tracking","project_mode":"greenfield"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "scope_workflow", Kind: domain.KindScopeItem, Title: "No grading", Body: json.RawMessage(`{"statement":"Automated grading is excluded","disposition":"out_of_scope","rationale":"Outside the request"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "assumption_workflow", Kind: domain.KindAssumption, Title: "Single school", Body: json.RawMessage(`{"statement":"One school uses the first release","impact_if_false":"medium","validation_method":"Ask the user","owner":"user"}`), Status: domain.EntityProposed, Origin: domain.OriginAgent, Confidence: &confidence}},
	}
	envelope, _ := json.Marshal(application.CommandEnvelope{ExpectedRevision: 2, Actor: "user", Commands: commands})
	modelResponse := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", string(envelope))
	if modelResponse.Code != http.StatusOK {
		t.Fatalf("model commands status = %d, body = %s", modelResponse.Code, modelResponse.Body.String())
	}

	contextState := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/workflow/continue", `{"expected_revision":3}`)
	decodeResponse(t, contextState, &state)
	if state.Stage != domain.StageContext || state.ProjectRevision != 4 || !state.GatePassed {
		t.Fatalf("context state = %#v", state)
	}
	scenariosState := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/workflow/continue", `{"expected_revision":4}`)
	decodeResponse(t, scenariosState, &state)
	if state.Stage != domain.StageScenarios || state.ProjectRevision != 5 {
		t.Fatalf("scenarios state = %#v", state)
	}

	reopened := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/workflow/reopen", `{"expected_revision":5,"stage":"FRAMING","reason":"New stakeholder evidence changes the boundary"}`)
	decodeResponse(t, reopened, &state)
	if state.Stage != domain.StageFraming || state.ProjectRevision != 6 {
		t.Fatalf("reopened state = %#v", state)
	}
	snapshotResponse := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/snapshot", "")
	var snapshot domain.Snapshot
	decodeResponse(t, snapshotResponse, &snapshot)
	for _, entity := range snapshot.Entities {
		if entity.ID == "assumption_workflow" && entity.Freshness != domain.FreshnessPotentiallyStale {
			t.Fatalf("reopened agent entity freshness = %s", entity.Freshness)
		}
	}

	rows, err := db.Query(`SELECT payload_json FROM events WHERE project_id = ? AND type = 'workflow.stage_changed' ORDER BY sequence`, project.ID)
	if err != nil {
		t.Fatalf("query stage transition events: %v", err)
	}
	defer rows.Close()
	type transitionEvent struct {
		Previous      domain.ProjectStage       `json:"previous"`
		Next          domain.ProjectStage       `json:"next"`
		GateChecks    []workflowmodel.GateCheck `json:"gate_checks"`
		Unresolved    []string                  `json:"unresolved"`
		ModelRevision int64                     `json:"model_revision"`
		Actor         string                    `json:"actor"`
		Reopened      bool                      `json:"reopened"`
		Reason        string                    `json:"reason"`
	}
	transitions := make([]transitionEvent, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan stage transition event: %v", err)
		}
		var event transitionEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("decode stage transition event: %v", err)
		}
		transitions = append(transitions, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate stage transition events: %v", err)
	}
	if len(transitions) != 4 {
		t.Fatalf("stage transition event count = %d, want 4", len(transitions))
	}
	first := transitions[0]
	if first.Previous != domain.StageIntake || first.Next != domain.StageFraming || len(first.GateChecks) != 2 || len(first.Unresolved) != 0 || first.ModelRevision != 2 || first.Actor != "workflow" || first.Reopened || first.Reason == "" {
		t.Fatalf("first stage transition event = %#v", first)
	}
	last := transitions[len(transitions)-1]
	if last.Previous != domain.StageScenarios || last.Next != domain.StageFraming || len(last.GateChecks) != 0 || len(last.Unresolved) != 0 || last.ModelRevision != 6 || !last.Reopened || last.Reason != "New stakeholder evidence changes the boundary" {
		t.Fatalf("reopen stage transition event = %#v", last)
	}
}
