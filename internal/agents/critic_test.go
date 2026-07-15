package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yjydist/traceframe/internal/domain"
)

func TestCriticRequestContainsCanonicalContextWithoutRunHistory(t *testing.T) {
	run := domain.AgentRun{ID: "run_critic", ProjectID: "project_critic", Role: domain.RoleCritic, Task: "Find readiness blockers", PromptVersion: "critic.v1", Budget: domain.DefaultRunBudget()}
	snapshot := domain.Snapshot{Project: domain.Project{ID: "project_critic", Stage: domain.StageReview}, Entities: []domain.Entity{{ID: "goal_critic", Kind: domain.KindGoal, Status: domain.EntityConfirmed, Freshness: domain.FreshnessCurrent}, {ID: "stale_omitted", Kind: domain.KindRisk, Status: domain.EntityConfirmed, Freshness: domain.FreshnessStale}}}
	request, err := BuildCriticRequest(run, snapshot)
	if err != nil {
		t.Fatalf("BuildCriticRequest() error = %v", err)
	}
	if len(request.Messages) != 2 || !strings.Contains(request.Messages[0].Content, "independent critic") {
		t.Fatalf("critic messages = %#v", request.Messages)
	}
	var context criticContext
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &context); err != nil {
		t.Fatalf("decode critic context: %v", err)
	}
	if len(context.Entities) != 1 || context.Entities[0].ID != "goal_critic" || strings.Contains(request.Messages[1].Content, "model_identifier") || strings.Contains(request.Messages[1].Content, "provider_request_id") {
		t.Fatalf("critic context includes noncanonical run data: %#v", context)
	}
}
