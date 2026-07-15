package application

import (
	"context"

	"github.com/yjydist/traceframe/internal/domain"
)

type TraceNode struct {
	ID        string              `json:"id"`
	Kind      domain.EntityKind   `json:"kind"`
	Title     string              `json:"title"`
	Status    domain.EntityStatus `json:"status"`
	Freshness domain.Freshness    `json:"freshness"`
	Incoming  int                 `json:"incoming"`
	Outgoing  int                 `json:"outgoing"`
}

type Traceability struct {
	ProjectRevision int64             `json:"project_revision"`
	Nodes           []TraceNode       `json:"nodes"`
	Edges           []domain.Relation `json:"edges"`
	Unlinked        []string          `json:"unlinked"`
}

func (s *ProjectService) Traceability(ctx context.Context, projectID string) (Traceability, error) {
	snapshot, err := s.store.GetSnapshot(ctx, projectID)
	if err != nil {
		return Traceability{}, err
	}
	counts := make(map[string]*[2]int, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		counts[entity.ID] = &[2]int{}
	}
	for _, relation := range snapshot.Relations {
		counts[relation.FromID][1]++
		counts[relation.ToID][0]++
	}
	nodes := make([]TraceNode, 0, len(snapshot.Entities))
	unlinked := make([]string, 0)
	for _, entity := range snapshot.Entities {
		count := counts[entity.ID]
		nodes = append(nodes, TraceNode{ID: entity.ID, Kind: entity.Kind, Title: entity.Title, Status: entity.Status, Freshness: entity.Freshness, Incoming: count[0], Outgoing: count[1]})
		if count[0] == 0 && count[1] == 0 {
			unlinked = append(unlinked, entity.ID)
		}
	}
	return Traceability{ProjectRevision: snapshot.Project.Revision, Nodes: nodes, Edges: snapshot.Relations, Unlinked: unlinked}, nil
}
