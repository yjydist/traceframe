package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

func TestQuestionPriorityBatchAndAnswerEvidence(t *testing.T) {
	handler, db := newProjectTestHandler(t)
	defer db.Close()
	create := performJSON(t, handler, http.MethodPost, "/api/v1/projects", `{"name":"Questions","raw_request":"Frame a student planner","mode":"greenfield"}`)
	var project domain.Project
	decodeResponse(t, create, &project)

	commands := make([]application.Command, 0, 4)
	for index, impact := range []int{1, 5, 3, 4} {
		body, _ := json.Marshal(map[string]any{"prompt": "Question", "reason": "Changes scope", "answer_type": "text", "impact": impact, "uncertainty": 5, "irreversibility": 2, "blocking": false, "disposition": "open"})
		confidence := 0.7
		commands = append(commands, application.Command{Type: "create_entity", Entity: &application.EntityDraft{ID: "qst_priority_" + string(rune('a'+index)), Kind: domain.KindQuestion, Title: "Question", Body: body, Status: domain.EntityProposed, Origin: domain.OriginAgent, Confidence: &confidence}})
	}
	envelope, _ := json.Marshal(application.CommandEnvelope{ExpectedRevision: 1, Actor: "agent:discovery", Commands: commands})
	created := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/commands", string(envelope))
	if created.Code != http.StatusOK {
		t.Fatalf("create questions status = %d, body = %s", created.Code, created.Body.String())
	}

	response := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/questions", "")
	var batch struct {
		Questions []application.QuestionItem `json:"questions"`
	}
	decodeResponse(t, response, &batch)
	if len(batch.Questions) != 3 || batch.Questions[0].Priority != 50 || batch.Questions[1].Priority != 40 || batch.Questions[2].Priority != 30 {
		t.Fatalf("question batch priorities = %#v", batch.Questions)
	}

	questionID := batch.Questions[0].Entity.ID
	answer := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/questions/"+questionID+"/answer", `{"expected_revision":2,"action":"answer","answer":"High school students"}`)
	if answer.Code != http.StatusOK {
		t.Fatalf("answer status = %d, body = %s", answer.Code, answer.Body.String())
	}
	var snapshot domain.Snapshot
	decodeResponse(t, answer, &snapshot)
	if snapshot.Project.Revision != 3 || len(snapshot.Relations) != 1 {
		t.Fatalf("answer snapshot = %#v", snapshot)
	}
	var answered *domain.Entity
	var answerEvidence int
	for index := range snapshot.Entities {
		entity := &snapshot.Entities[index]
		if entity.ID == questionID {
			answered = entity
		}
		if entity.Kind == domain.KindEvidence && entity.ID != snapshot.Entities[0].ID {
			answerEvidence++
		}
	}
	if answered == nil || answered.Status != domain.EntityConfirmed || answerEvidence != 1 {
		t.Fatalf("answered question = %#v, answer evidence = %d", answered, answerEvidence)
	}

	remaining := performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/questions", "")
	decodeResponse(t, remaining, &batch)
	deferredID := batch.Questions[0].Entity.ID
	deferred := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/questions/"+deferredID+"/answer", `{"expected_revision":3,"action":"defer"}`)
	if deferred.Code != http.StatusOK {
		t.Fatalf("defer status = %d, body = %s", deferred.Code, deferred.Body.String())
	}
	remaining = performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/questions", "")
	decodeResponse(t, remaining, &batch)
	rejectedID := batch.Questions[0].Entity.ID
	rejected := performJSON(t, handler, http.MethodPost, "/api/v1/projects/"+project.ID+"/questions/"+rejectedID+"/answer", `{"expected_revision":4,"action":"reject"}`)
	if rejected.Code != http.StatusOK {
		t.Fatalf("reject status = %d, body = %s", rejected.Code, rejected.Body.String())
	}
	remaining = performJSON(t, handler, http.MethodGet, "/api/v1/projects/"+project.ID+"/questions", "")
	decodeResponse(t, remaining, &batch)
	if len(batch.Questions) != 1 || batch.Questions[0].Priority != 10 {
		t.Fatalf("remaining questions after answer/defer/reject = %#v", batch.Questions)
	}
}
