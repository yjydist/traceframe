package agents

import (
	"encoding/json"
	"testing"

	"github.com/yjydist/traceframe/internal/domain"
)

func TestSpecialistRequestUsesRoleContractAndMinimumContext(t *testing.T) {
	run := domain.AgentRun{ID: "run_delivery", ProjectID: "project_1", Role: domain.RoleDelivery, Task: "Create one verified vertical slice", PromptVersion: "delivery.v1", Budget: domain.DefaultRunBudget()}
	snapshot := domain.Snapshot{Project: domain.Project{ID: "project_1", Stage: domain.StageDelivery}, Entities: []domain.Entity{
		{ID: "goal_1", Kind: domain.KindGoal, Status: domain.EntityConfirmed, Freshness: domain.FreshnessCurrent},
		{ID: "requirement_1", Kind: domain.KindRequirement, Status: domain.EntityConfirmed, Freshness: domain.FreshnessCurrent},
		{ID: "scenario_omitted", Kind: domain.KindScenario, Status: domain.EntityConfirmed, Freshness: domain.FreshnessCurrent},
		{ID: "stale_omitted", Kind: domain.KindRisk, Status: domain.EntityConfirmed, Freshness: domain.FreshnessStale},
	}}
	request, err := BuildProposalRequest(run, snapshot)
	if err != nil {
		t.Fatalf("BuildProposalRequest() error = %v", err)
	}
	if len(request.Messages) != 2 || len(request.ResponseSchema) == 0 || request.Metadata["role"] != "delivery" {
		t.Fatalf("request = %#v", request)
	}
	var context specialistContext
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &context); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if context.RoleContract == "" || context.StageGate == "" || len(context.Entities) != 2 {
		t.Fatalf("specialist context = %#v", context)
	}
}
