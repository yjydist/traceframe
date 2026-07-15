package domain

import (
	"fmt"
	"strings"
)

var relationKinds = map[RelationType]struct{}{
	RelationMotivates: {}, RelationAffects: {}, RelationConstrains: {}, RelationAssumes: {}, RelationAnswers: {}, RelationDerivesFrom: {},
	RelationSatisfies: {}, RelationVerifies: {}, RelationMitigates: {}, RelationSelects: {}, RelationRejects: {}, RelationDependsOn: {},
	RelationConflictsWith: {}, RelationSupersedes: {}, RelationImplements: {}, RelationDecomposes: {}, RelationEvidencedBy: {},
}

func ValidateRelation(relation Relation, from, to Entity) error {
	if strings.TrimSpace(relation.ID) == "" || strings.TrimSpace(relation.ProjectID) == "" || strings.TrimSpace(relation.FromID) == "" || strings.TrimSpace(relation.ToID) == "" {
		return fmt.Errorf("%w: relation id, project_id, from_id, and to_id are required", ErrInvalid)
	}
	if _, ok := relationKinds[relation.Type]; !ok {
		return fmt.Errorf("%w: unsupported relation type %q", ErrInvalid, relation.Type)
	}
	if from.ProjectID != relation.ProjectID || to.ProjectID != relation.ProjectID {
		return fmt.Errorf("%w: relation endpoints must belong to the same project", ErrInvalid)
	}
	if from.ID != relation.FromID || to.ID != relation.ToID {
		return fmt.Errorf("%w: relation endpoint ids do not match supplied entities", ErrInvalid)
	}
	if from.ID == to.ID && relation.Type != RelationSupersedes {
		return fmt.Errorf("%w: relation cannot point to itself", ErrInvalid)
	}
	if !compatibleRelation(relation.Type, from.Kind, to.Kind) {
		return fmt.Errorf("%w: %s cannot %s %s", ErrInvalid, from.Kind, relation.Type, to.Kind)
	}
	return nil
}

func compatibleRelation(relationType RelationType, from, to EntityKind) bool {
	switch relationType {
	case RelationVerifies:
		return from == KindVerification && oneOf(to, KindRequirement, KindQualityScenario, KindGoal, KindDecision, KindWorkSlice)
	case RelationSelects, RelationRejects:
		return from == KindDecision && to == KindOption
	case RelationEvidencedBy:
		return to == KindEvidence && from != KindEvidence
	case RelationMitigates:
		return oneOf(from, KindDecision, KindWorkSlice, KindExperiment, KindVerification) && to == KindRisk
	case RelationAnswers:
		return to == KindQuestion && from != KindQuestion
	case RelationSatisfies:
		return oneOf(from, KindScenario, KindRequirement, KindWorkSlice) && oneOf(to, KindGoal, KindScenario, KindRequirement)
	case RelationImplements:
		return oneOf(from, KindWorkSlice, KindSystemElement) && oneOf(to, KindRequirement, KindQualityScenario, KindDecision)
	case RelationMotivates:
		return oneOf(from, KindGoal, KindStakeholder, KindContext, KindRisk) && from != to
	case RelationConstrains:
		return from == KindConstraint && to != KindEvidence
	case RelationAssumes:
		return to == KindAssumption && from != KindAssumption
	case RelationDecomposes:
		return from == to && oneOf(from, KindGoal, KindScenario, KindRequirement, KindSystemElement, KindWorkSlice)
	case RelationSupersedes:
		return from == to
	case RelationAffects, RelationDerivesFrom, RelationDependsOn, RelationConflictsWith:
		return true
	default:
		return false
	}
}

func oneOf(value EntityKind, options ...EntityKind) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
