package agents

import (
	"encoding/json"
	"testing"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

func TestDiscoveryProposalAuthority(t *testing.T) {
	proposal := Proposal{RunID: "run_1", BaseRevision: 1, Summary: "Frame the outcome", Commands: []application.Command{{
		Type: "create_entity", Entity: &application.EntityDraft{ID: "goal_1", Kind: domain.KindGoal, Title: "Outcome", Body: json.RawMessage(`{"outcome":"Useful result","success_signals":[],"priority":"must"}`), SourceRefs: []string{"evidence_1"}},
	}}}
	if err := ValidateProposal(domain.RoleDiscovery, proposal); err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}
	NormalizeProposal(&proposal)
	if proposal.Commands[0].Entity.Status != domain.EntityProposed || proposal.Commands[0].Entity.Origin != domain.OriginAgent {
		t.Fatalf("proposal not normalized: %#v", proposal.Commands[0].Entity)
	}

	proposal.Commands[0].Entity.Kind = domain.KindDecision
	if err := ValidateProposal(domain.RoleDiscovery, proposal); err == nil {
		t.Fatal("discovery created an unauthorized decision")
	}
}

func TestSpecialistRoleAuthority(t *testing.T) {
	tests := []struct {
		role    domain.AgentRole
		allowed domain.EntityKind
		denied  domain.EntityKind
	}{
		{domain.RoleRequirements, domain.KindRequirement, domain.KindDecision},
		{domain.RoleArchitecture, domain.KindDecision, domain.KindWorkSlice},
		{domain.RoleQualityRisk, domain.KindRisk, domain.KindDecision},
		{domain.RoleDelivery, domain.KindWorkSlice, domain.KindOption},
	}
	for _, test := range tests {
		t.Run(string(test.role), func(t *testing.T) {
			proposal := Proposal{RunID: "run_role", BaseRevision: 1, Summary: "Bounded proposal", Commands: []application.Command{{
				Type: "create_entity", Entity: &application.EntityDraft{ID: "entity_role", Kind: test.allowed, SourceRefs: []string{"evidence_1"}},
			}}}
			if err := ValidateProposal(test.role, proposal); err != nil {
				t.Fatalf("allowed kind rejected: %v", err)
			}
			proposal.Commands[0].Entity.Kind = test.denied
			if err := ValidateProposal(test.role, proposal); err == nil {
				t.Fatalf("denied kind %s accepted", test.denied)
			}
		})
	}
}
