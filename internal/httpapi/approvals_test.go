package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

func TestDecisionApprovalUsesExactRevisionAndImpactInvalidatesIt(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()
	create := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Approval","raw_request":"Choose a significant boundary","mode":"greenfield"}`)
	var project domain.Project
	decodeResponse(t, create, &project)
	confidence := 1.0
	commands := []application.Command{
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "option_approval", Kind: domain.KindOption, Title: "Modular boundary", Body: json.RawMessage(`{"decision_topic":"Boundary","description":"Use a modular boundary","benefits":["Isolation"],"costs":["Coordination"],"risks":["Wrong boundary"],"fit_to_constraints":[],"evidence_refs":[]}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "decision_approval", Kind: domain.KindDecision, Title: "Select boundary", Body: json.RawMessage(`{"question":"Which boundary?","selected_option_id":"option_approval","rationale":"The boundary isolates change","consequences":["A stable interface is required"],"alternatives_considered":["Single module"],"revisit_triggers":["Coupling increases"],"significance":"architectural","approval_required":true}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "risk_approval", Kind: domain.KindRisk, Title: "Boundary risk", Body: json.RawMessage(`{"category":"architecture","cause":"The boundary is misplaced","event":"Changes cross the boundary","impact":"Coupling increases","likelihood":"medium","severity":"high","mitigation":"Measure cross-boundary changes","evidence_needed":["Change data"],"residual_risk":"medium"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_unrelated", Kind: domain.KindGoal, Title: "Unrelated goal", Body: json.RawMessage(`{"outcome":"Preserve an unrelated outcome","success_signals":["Outcome remains current"],"priority":"should"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence}},
		{Type: "create_relation", Relation: &application.RelationDraft{ID: "rel_approval", FromID: "decision_approval", Type: domain.RelationSelects, ToID: "option_approval", Rationale: "The decision selects the option"}},
		{Type: "create_relation", Relation: &application.RelationDraft{ID: "rel_approval_risk", FromID: "option_approval", Type: domain.RelationAffects, ToID: "risk_approval", Rationale: "The selected boundary affects the coupling risk"}},
	}
	envelope, _ := json.Marshal(application.CommandEnvelope{ExpectedRevision: 1, Commands: commands})
	response := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", string(envelope))
	if response.Code != http.StatusOK {
		t.Fatalf("create decision status = %d, body = %s", response.Code, response.Body.String())
	}

	response = performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/approvals", "")
	var listed struct {
		Approvals []domain.Approval `json:"approvals"`
	}
	decodeResponse(t, response, &listed)
	if len(listed.Approvals) != 1 || listed.Approvals[0].SubjectRevision != 1 || listed.Approvals[0].ProjectRevision != 2 || listed.Approvals[0].Status != domain.ApprovalPending {
		t.Fatalf("pending approvals = %#v", listed.Approvals)
	}
	approval := listed.Approvals[0]
	stale := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/approvals/"+approval.ID+"/approve", `{"expected_revision":1}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale approval status = %d, body = %s", stale.Code, stale.Body.String())
	}
	approved := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/approvals/"+approval.ID+"/approve", `{"expected_revision":2,"rationale":"The trade-off is understood"}`)
	if approved.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", approved.Code, approved.Body.String())
	}

	updatedBody := `{"question":"Which boundary?","selected_option_id":"option_approval","rationale":"New evidence changes the boundary rationale","consequences":["A stable interface is required"],"alternatives_considered":["Single module"],"revisit_triggers":["Coupling increases"],"significance":"architectural","approval_required":true}`
	updateEnvelope, _ := json.Marshal(application.CommandEnvelope{ExpectedRevision: 2, Commands: []application.Command{{Type: "update_entity", EntityID: "decision_approval", ExpectedEntityRevision: 1, Changes: &application.EntityChanges{Body: json.RawMessage(updatedBody)}}}})
	response = performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", string(updateEnvelope))
	var snapshot domain.Snapshot
	decodeResponse(t, response, &snapshot)
	for _, entity := range snapshot.Entities {
		switch entity.ID {
		case "option_approval", "risk_approval":
			if entity.Freshness != domain.FreshnessPotentiallyStale || entity.Revision != 2 {
				t.Fatalf("impacted entity = %#v", entity)
			}
		case "goal_unrelated":
			if entity.Freshness != domain.FreshnessCurrent || entity.Revision != 1 {
				t.Fatalf("unrelated entity changed = %#v", entity)
			}
		}
	}
	response = performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/approvals", "")
	decodeResponse(t, response, &listed)
	if len(listed.Approvals) != 2 || listed.Approvals[0].Status != domain.ApprovalInvalidated || listed.Approvals[1].SubjectRevision != 2 || listed.Approvals[1].Status != domain.ApprovalPending {
		t.Fatalf("approvals after impact = %#v", listed.Approvals)
	}
}
