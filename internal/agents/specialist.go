package agents

import (
	"encoding/json"
	"fmt"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	traceprompts "github.com/yjydist/traceframe/prompts"
)

type specialistContext struct {
	RunID        string            `json:"run_id"`
	Task         string            `json:"task"`
	Role         domain.AgentRole  `json:"role"`
	RoleContract string            `json:"role_contract"`
	Project      domain.Project    `json:"project"`
	Entities     []domain.Entity   `json:"entities"`
	Relations    []domain.Relation `json:"relations"`
	StageGate    string            `json:"stage_gate"`
}

func BuildProposalRequest(run domain.AgentRun, snapshot domain.Snapshot) (models.GenerateRequest, error) {
	if run.Role == domain.RoleDiscovery {
		return BuildDiscoveryRequest(run, selectedSnapshot(run.Role, snapshot))
	}
	if run.Role == domain.RoleCritic {
		return BuildCriticRequest(run, snapshot)
	}
	system, responseSchema, err := traceprompts.SpecialistV1()
	if err != nil {
		return models.GenerateRequest{}, fmt.Errorf("load specialist prompt: %w", err)
	}
	selected := selectedSnapshot(run.Role, snapshot)
	contextPack := specialistContext{
		RunID: run.ID, Task: run.Task, Role: run.Role, RoleContract: roleContract(run.Role), Project: snapshot.Project,
		Entities: selected.Entities, Relations: selected.Relations, StageGate: stageGate(snapshot.Project.Stage),
	}
	contextJSON, err := json.Marshal(contextPack)
	if err != nil {
		return models.GenerateRequest{}, fmt.Errorf("encode specialist context: %w", err)
	}
	return models.GenerateRequest{
		Messages: []models.Message{{Role: "system", Content: system}, {Role: "user", Content: string(contextJSON)}},
		Tools:    []models.ToolSchema{}, ResponseSchema: responseSchema, TemperaturePolicy: models.TemperatureDeterministic,
		TokenBudget: models.TokenBudget{MaxInputTokens: run.Budget.MaxInputTokens, MaxOutputTokens: run.Budget.MaxOutputTokens},
		Metadata:    map[string]string{"run_id": run.ID, "project_id": run.ProjectID, "role": string(run.Role), "prompt_version": run.PromptVersion},
	}, nil
}

func SelectContextIDs(role domain.AgentRole, snapshot domain.Snapshot) []string {
	selected := selectedSnapshot(role, snapshot)
	ids := make([]string, 0, len(selected.Entities))
	for _, entity := range selected.Entities {
		ids = append(ids, entity.ID)
	}
	return ids
}

func selectedSnapshot(role domain.AgentRole, snapshot domain.Snapshot) domain.Snapshot {
	wanted := contextKinds(role)
	selected := domain.Snapshot{SchemaVersion: snapshot.SchemaVersion, Project: snapshot.Project, Entities: []domain.Entity{}, Relations: []domain.Relation{}}
	ids := make(map[string]struct{})
	for _, entity := range snapshot.Entities {
		if entity.Freshness == domain.FreshnessStale || entity.Status == domain.EntityRejected || entity.Status == domain.EntitySuperseded {
			continue
		}
		if _, ok := wanted[entity.Kind]; !ok && entity.Kind != domain.KindEvidence && entity.Kind != domain.KindQuestion {
			continue
		}
		selected.Entities = append(selected.Entities, entity)
		ids[entity.ID] = struct{}{}
	}
	for _, relation := range snapshot.Relations {
		_, from := ids[relation.FromID]
		_, to := ids[relation.ToID]
		if from && to {
			selected.Relations = append(selected.Relations, relation)
		}
	}
	return selected
}

func contextKinds(role domain.AgentRole) map[domain.EntityKind]struct{} {
	switch role {
	case domain.RoleRequirements:
		return kinds(domain.KindGoal, domain.KindStakeholder, domain.KindContext, domain.KindScopeItem, domain.KindConstraint, domain.KindAssumption, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindTerm, domain.KindRisk, domain.KindVerification)
	case domain.RoleArchitecture:
		return kinds(domain.KindGoal, domain.KindContext, domain.KindConstraint, domain.KindAssumption, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindRisk, domain.KindOption, domain.KindDecision, domain.KindSystemElement, domain.KindExperiment)
	case domain.RoleQualityRisk:
		return kinds(domain.KindGoal, domain.KindContext, domain.KindConstraint, domain.KindAssumption, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindRisk, domain.KindOption, domain.KindDecision, domain.KindSystemElement, domain.KindExperiment)
	case domain.RoleDelivery:
		return kinds(domain.KindGoal, domain.KindScopeItem, domain.KindConstraint, domain.KindRequirement, domain.KindQualityScenario, domain.KindRisk, domain.KindDecision, domain.KindSystemElement, domain.KindWorkSlice, domain.KindVerification)
	case domain.RoleCritic:
		return kinds(domain.KindGoal, domain.KindStakeholder, domain.KindContext, domain.KindScopeItem, domain.KindConstraint, domain.KindAssumption, domain.KindQuestion, domain.KindTerm, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindRisk, domain.KindOption, domain.KindDecision, domain.KindSystemElement, domain.KindWorkSlice, domain.KindExperiment, domain.KindVerification)
	default:
		return discoveryKinds
	}
}

func roleContract(role domain.AgentRole) string {
	switch role {
	case domain.RoleRequirements:
		return "Derive normal, alternative, failure, and edge scenarios; traced verifiable requirements; measurable quality scenarios; glossary terms; and verification obligations."
	case domain.RoleArchitecture:
		return "Shape boundaries and responsibilities, preserve visible alternatives, compare options against constraints and evidence, and record significant decisions with consequences and revisit triggers."
	case domain.RoleQualityRisk:
		return "Challenge security, privacy, reliability, performance, operations, compatibility, and feasibility; propose measurable quality scenarios, risks, mitigations, or bounded experiments."
	case domain.RoleDelivery:
		return "Create ordered vertical work slices that implement confirmed requirements, reduce risk, and name explicit verification and completion evidence."
	case domain.RoleCritic:
		return "Independently identify contradictions, omissions, weak evidence, and over-design without rewriting the project model."
	default:
		return "Perform only the bounded task authorized for the role."
	}
}

func stageGate(stage domain.ProjectStage) string {
	switch stage {
	case domain.StageScenarios:
		return "Priority user and system scenarios cover confirmed goals."
	case domain.StageRequirements:
		return "Confirmed requirements are traced and verifiable, and important quality concerns are addressed."
	case domain.StageShaping:
		return "The macro solution is coherent and bounded, with major feasibility risks treated."
	case domain.StageDecisions:
		return "Significant decisions preserve alternatives, evidence, consequences, and exact-revision approval requirements."
	case domain.StageDelivery:
		return "Ordered slices cover approved scope, dependencies, completion conditions, and verification."
	default:
		return "Reduce only gaps relevant to the current stage."
	}
}
