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
