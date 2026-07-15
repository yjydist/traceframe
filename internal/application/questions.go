package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

type QuestionItem struct {
	Entity   domain.Entity `json:"entity"`
	Priority int           `json:"priority"`
	Reason   string        `json:"reason"`
	Blocking bool          `json:"blocking"`
}

type QuestionResponse struct {
	ExpectedRevision int64           `json:"expected_revision"`
	Action           string          `json:"action"`
	Answer           json.RawMessage `json:"answer,omitempty"`
}

func (s *ProjectService) Questions(ctx context.Context, projectID string) ([]QuestionItem, error) {
	snapshot, err := s.store.GetSnapshot(ctx, projectID)
	if err != nil {
		return nil, err
	}
	questions := make([]QuestionItem, 0)
	for _, entity := range snapshot.Entities {
		if entity.Kind != domain.KindQuestion || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded {
			continue
		}
		body, err := questionBody(entity.Body)
		if err != nil {
			return nil, err
		}
		disposition, _ := body["disposition"].(string)
		if disposition == "answered" || disposition == "deferred" || disposition == "rejected" {
			continue
		}
		impact, _ := body["impact"].(json.Number).Int64()
		uncertainty, _ := body["uncertainty"].(json.Number).Int64()
		irreversibility, _ := body["irreversibility"].(json.Number).Int64()
		reason, _ := body["reason"].(string)
		blocking, _ := body["blocking"].(bool)
		questions = append(questions, QuestionItem{Entity: entity, Priority: int(impact * uncertainty * irreversibility), Reason: reason, Blocking: blocking})
	}
	sort.SliceStable(questions, func(i, j int) bool {
		if questions[i].Priority == questions[j].Priority {
			return questions[i].Entity.ID < questions[j].Entity.ID
		}
		return questions[i].Priority > questions[j].Priority
	})
	if len(questions) > 3 {
		questions = questions[:3]
	}
	return questions, nil
}

func (s *ProjectService) RespondToQuestion(ctx context.Context, projectID, questionID string, response QuestionResponse) (domain.Snapshot, error) {
	if response.ExpectedRevision < 1 {
		return domain.Snapshot{}, fmt.Errorf("%w: expected_revision must be positive", domain.ErrInvalid)
	}
	snapshot, err := s.store.GetSnapshot(ctx, projectID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	var question domain.Entity
	found := false
	for _, entity := range snapshot.Entities {
		if entity.ID == questionID && entity.Kind == domain.KindQuestion {
			question, found = entity, true
			break
		}
	}
	if !found {
		return domain.Snapshot{}, fmt.Errorf("%w: question %s", ErrNotFound, questionID)
	}
	body, err := questionBody(question.Body)
	if err != nil {
		return domain.Snapshot{}, err
	}
	commands := make([]Command, 0, 3)
	status := domain.EntityUnresolved
	switch response.Action {
	case "answer":
		if len(response.Answer) == 0 || string(response.Answer) == "null" {
			return domain.Snapshot{}, fmt.Errorf("%w: answer is required", domain.ErrInvalid)
		}
		var answer any
		decoder := json.NewDecoder(bytes.NewReader(response.Answer))
		decoder.UseNumber()
		if err := decoder.Decode(&answer); err != nil {
			return domain.Snapshot{}, fmt.Errorf("%w: invalid answer", domain.ErrInvalid)
		}
		body["answer"], body["disposition"] = answer, "answered"
		status = domain.EntityConfirmed
	case "defer":
		body["disposition"] = "deferred"
	case "reject":
		body["disposition"] = "rejected"
		status = domain.EntityRejected
	default:
		return domain.Snapshot{}, fmt.Errorf("%w: action must be answer, defer, or reject", domain.ErrInvalid)
	}
	bodyJSON, _ := json.Marshal(body)
	commands = append(commands, Command{Type: "update_entity", EntityID: question.ID, ExpectedEntityRevision: question.Revision, Changes: &EntityChanges{Body: bodyJSON, Status: &status}})
	if response.Action == "answer" {
		now := s.store.Now()
		evidenceID := domain.NewID("evidence")
		evidenceBody, _ := json.Marshal(map[string]any{"evidence_type": "user_statement", "summary": "User answered question " + question.ID, "locator": "question:" + question.ID, "captured_at": now.Format(time.RFC3339Nano), "freshness": "current", "trust_notes": "Captured from the explicit question response."})
		confidence := 1.0
		commands = append(commands,
			Command{Type: "create_entity", Entity: &EntityDraft{ID: evidenceID, Kind: domain.KindEvidence, Title: "Answer to " + question.Title, Body: evidenceBody, Status: domain.EntityConfirmed, Origin: domain.OriginUser, Confidence: &confidence, SourceRefs: []string{}, Tags: []string{"question_answer"}}},
			Command{Type: "create_relation", Relation: &RelationDraft{ID: domain.NewID("rel"), FromID: evidenceID, Type: domain.RelationAnswers, ToID: question.ID, Rationale: "The user statement directly answers this question."}},
		)
	}
	return s.ApplyCommands(ctx, projectID, CommandEnvelope{ExpectedRevision: response.ExpectedRevision, Actor: "user", Commands: commands})
}

func questionBody(raw json.RawMessage) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		return nil, fmt.Errorf("decode question body: %w", err)
	}
	return body, nil
}
