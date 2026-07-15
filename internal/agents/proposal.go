package agents

import (
	"fmt"
	"strings"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

type Proposal struct {
	RunID                 string                `json:"run_id"`
	BaseRevision          int64                 `json:"base_revision"`
	Summary               string                `json:"summary"`
	Commands              []application.Command `json:"commands"`
	Warnings              []string              `json:"warnings"`
	Unresolved            []string              `json:"unresolved"`
	RecommendedNextAction string                `json:"recommended_next_action"`
}

var discoveryKinds = map[domain.EntityKind]struct{}{
	domain.KindGoal: {}, domain.KindStakeholder: {}, domain.KindContext: {}, domain.KindScopeItem: {},
	domain.KindConstraint: {}, domain.KindAssumption: {}, domain.KindQuestion: {}, domain.KindTerm: {},
}

var roleKinds = map[domain.AgentRole]map[domain.EntityKind]struct{}{
	domain.RoleDiscovery:    discoveryKinds,
	domain.RoleRequirements: kinds(domain.KindScenario, domain.KindRequirement, domain.KindQualityScenario, domain.KindTerm, domain.KindQuestion, domain.KindVerification),
	domain.RoleArchitecture: kinds(domain.KindSystemElement, domain.KindOption, domain.KindDecision, domain.KindRisk, domain.KindExperiment, domain.KindQuestion),
	domain.RoleQualityRisk:  kinds(domain.KindRisk, domain.KindQualityScenario, domain.KindExperiment, domain.KindConstraint, domain.KindQuestion),
	domain.RoleDelivery:     kinds(domain.KindWorkSlice, domain.KindVerification, domain.KindRisk, domain.KindQuestion),
}

func kinds(values ...domain.EntityKind) map[domain.EntityKind]struct{} {
	result := make(map[domain.EntityKind]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func ValidateProposal(role domain.AgentRole, proposal Proposal) error {
	if strings.TrimSpace(proposal.RunID) == "" || proposal.BaseRevision < 1 || strings.TrimSpace(proposal.Summary) == "" {
		return fmt.Errorf("%w: run_id, base_revision, and summary are required", domain.ErrInvalid)
	}
	if len(proposal.Commands) == 0 || len(proposal.Commands) > 100 {
		return fmt.Errorf("%w: proposal must contain between 1 and 100 commands", domain.ErrInvalid)
	}
	allowedKinds, roleEnabled := roleKinds[role]
	if !roleEnabled {
		return fmt.Errorf("%w: role %q cannot submit model proposals", domain.ErrInvalid, role)
	}
	for index, command := range proposal.Commands {
		switch command.Type {
		case "create_entity":
			if command.Entity == nil {
				return fmt.Errorf("%w: command %d has no entity", domain.ErrInvalid, index)
			}
			if _, allowed := allowedKinds[command.Entity.Kind]; !allowed {
				return fmt.Errorf("%w: %s cannot create %s", domain.ErrInvalid, role, command.Entity.Kind)
			}
			if strings.TrimSpace(command.Entity.ID) == "" {
				return fmt.Errorf("%w: agent-created entities require stable ids", domain.ErrInvalid)
			}
			if requiresEvidence(command.Entity.Kind) && len(command.Entity.SourceRefs) == 0 {
				return fmt.Errorf("%w: agent-created %s requires source_refs", domain.ErrInvalid, command.Entity.Kind)
			}
			if command.Entity.Status != "" && command.Entity.Status != domain.EntityProposed {
				return fmt.Errorf("%w: agents may only create proposed entities", domain.ErrInvalid)
			}
			if command.Entity.Origin != "" && command.Entity.Origin != domain.OriginAgent {
				return fmt.Errorf("%w: agent-created entities must have agent origin", domain.ErrInvalid)
			}
		case "create_relation":
			if command.Relation == nil {
				return fmt.Errorf("%w: command %d has no relation", domain.ErrInvalid, index)
			}
			if strings.TrimSpace(command.Relation.ID) == "" {
				return fmt.Errorf("%w: agent-created relations require stable ids", domain.ErrInvalid)
			}
		default:
			return fmt.Errorf("%w: %s command %q is not authorized", domain.ErrInvalid, role, command.Type)
		}
	}
	return nil
}

func requiresEvidence(kind domain.EntityKind) bool {
	return kind != domain.KindQuestion && kind != domain.KindAssumption
}

func NormalizeProposal(proposal *Proposal) {
	for index := range proposal.Commands {
		if entity := proposal.Commands[index].Entity; entity != nil {
			entity.Status = domain.EntityProposed
			entity.Origin = domain.OriginAgent
			entity.Freshness = domain.FreshnessCurrent
		}
	}
}
