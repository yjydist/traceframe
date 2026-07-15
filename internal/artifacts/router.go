package artifacts

import (
	"slices"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/workflow"
)

func Route(snapshot domain.Snapshot, concerns []workflow.RoutedConcern) []Definition {
	definitions := []Definition{
		{ViewType: "purpose_scope", Title: "Purpose and scope", Mandatory: true, Kinds: []domain.EntityKind{domain.KindGoal, domain.KindStakeholder, domain.KindContext, domain.KindScopeItem}},
		{ViewType: "facts_assumptions", Title: "Facts, constraints, and assumptions", Kinds: []domain.EntityKind{domain.KindEvidence, domain.KindConstraint, domain.KindAssumption, domain.KindContext, domain.KindTerm}},
		{ViewType: "questions", Title: "Open questions", Kinds: []domain.EntityKind{domain.KindQuestion}},
		{ViewType: "risks_decisions", Title: "Risks, options, and decisions", Kinds: []domain.EntityKind{domain.KindRisk, domain.KindOption, domain.KindDecision, domain.KindExperiment}},
		{ViewType: "traceability_readiness", Title: "Traceability and readiness", Mandatory: true, Kinds: []domain.EntityKind{domain.KindGoal, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindDecision, domain.KindWorkSlice, domain.KindVerification}},
		{ViewType: "delivery_verification", Title: "Delivery slices and verification", Kinds: []domain.EntityKind{domain.KindWorkSlice, domain.KindVerification, domain.KindRequirement}},
		{ViewType: "implementation_packet", Title: "Implementation handoff packet", Mandatory: true, Kinds: allKinds()},
	}
	conditional := map[string]Definition{
		"interaction":       {ViewType: "interaction", Title: "User flows and interaction states", Kinds: []domain.EntityKind{domain.KindStakeholder, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario}},
		"api":               {ViewType: "interface_contract", Title: "Interface contract", Kinds: []domain.EntityKind{domain.KindRequirement, domain.KindSystemElement, domain.KindDecision, domain.KindVerification}},
		"data":              {ViewType: "data_model", Title: "Conceptual data model", Kinds: []domain.EntityKind{domain.KindContext, domain.KindRequirement, domain.KindSystemElement, domain.KindConstraint}},
		"migration":         {ViewType: "migration_strategy", Title: "Migration and rollback strategy", Kinds: []domain.EntityKind{domain.KindContext, domain.KindConstraint, domain.KindRisk, domain.KindDecision, domain.KindWorkSlice, domain.KindVerification}},
		"runtime":           {ViewType: "runtime_interaction", Title: "Runtime interactions", Kinds: []domain.EntityKind{domain.KindScenario, domain.KindSystemElement, domain.KindQualityScenario, domain.KindRisk}},
		"deployment":        {ViewType: "deployment", Title: "Deployment view", Kinds: []domain.EntityKind{domain.KindContext, domain.KindSystemElement, domain.KindConstraint, domain.KindRisk}},
		"security":          {ViewType: "security", Title: "Security and trust boundaries", Kinds: []domain.EntityKind{domain.KindContext, domain.KindConstraint, domain.KindQualityScenario, domain.KindRisk, domain.KindDecision, domain.KindSystemElement, domain.KindVerification}},
		"privacy":           {ViewType: "privacy", Title: "Privacy treatment", Kinds: []domain.EntityKind{domain.KindContext, domain.KindConstraint, domain.KindQualityScenario, domain.KindRisk, domain.KindDecision}},
		"reliability":       {ViewType: "failure_recovery", Title: "Failure and recovery", Kinds: []domain.EntityKind{domain.KindScenario, domain.KindQualityScenario, domain.KindRisk, domain.KindDecision, domain.KindVerification}},
		"performance":       {ViewType: "performance", Title: "Performance model", Kinds: []domain.EntityKind{domain.KindConstraint, domain.KindQualityScenario, domain.KindRisk, domain.KindVerification}},
		"compatibility":     {ViewType: "compatibility", Title: "Version and compatibility strategy", Kinds: []domain.EntityKind{domain.KindContext, domain.KindConstraint, domain.KindRequirement, domain.KindRisk, domain.KindDecision, domain.KindVerification}},
		"repository_change": {ViewType: "current_state_impact", Title: "Current state and change impact", Kinds: []domain.EntityKind{domain.KindContext, domain.KindEvidence, domain.KindRisk, domain.KindWorkSlice, domain.KindVerification}},
		"experiment":        {ViewType: "experiment", Title: "Hypotheses and evidence", Kinds: []domain.EntityKind{domain.KindQuestion, domain.KindAssumption, domain.KindExperiment, domain.KindEvidence, domain.KindRisk}},
	}
	for _, concern := range concerns {
		definition, ok := conditional[concern.Name]
		if !ok {
			continue
		}
		definition.Concern = concern.Name
		definition.Mandatory = concern.Mandatory
		definitions = append(definitions, definition)
	}
	result := make([]Definition, 0, len(definitions))
	seen := make(map[string]struct{})
	for _, definition := range definitions {
		if _, exists := seen[definition.ViewType]; exists {
			continue
		}
		if definition.Mandatory || hasKinds(snapshot, definition.Kinds) {
			result = append(result, definition)
			seen[definition.ViewType] = struct{}{}
		}
	}
	slices.SortFunc(result, func(a, b Definition) int {
		if a.ViewType < b.ViewType {
			return -1
		}
		if a.ViewType > b.ViewType {
			return 1
		}
		return 0
	})
	return result
}

func hasKinds(snapshot domain.Snapshot, kinds []domain.EntityKind) bool {
	for _, entity := range snapshot.Entities {
		if (entity.Status == domain.EntityConfirmed || entity.Status == domain.EntityProposed || entity.Status == domain.EntityUnresolved) && entity.Freshness != domain.FreshnessStale && slices.Contains(kinds, entity.Kind) {
			return true
		}
	}
	return false
}

func allKinds() []domain.EntityKind {
	return []domain.EntityKind{domain.KindGoal, domain.KindStakeholder, domain.KindContext, domain.KindScopeItem, domain.KindConstraint, domain.KindAssumption, domain.KindQuestion, domain.KindTerm, domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindRisk, domain.KindOption, domain.KindDecision, domain.KindSystemElement, domain.KindWorkSlice, domain.KindExperiment, domain.KindEvidence, domain.KindVerification}
}
