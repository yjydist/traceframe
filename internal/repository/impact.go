package repository

import (
	"context"
	"fmt"
	"slices"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
)

func (s *Service) Impact(ctx context.Context, projectID, subjectID string) (ImpactAnalysis, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return ImpactAnalysis{}, err
	}
	if !repositoryMode(snapshot.Project.Mode) {
		return ImpactAnalysis{}, fmt.Errorf("%w: repository impact analysis is available only for feature and refactor projects", domain.ErrInvalid)
	}
	entities := make(map[string]domain.Entity, len(snapshot.Entities))
	repositoryEvidence := make(map[string]struct{})
	stale := make([]string, 0)
	for _, entity := range snapshot.Entities {
		entities[entity.ID] = entity
		if entity.Kind == domain.KindEvidence && entity.Origin == domain.OriginRepository {
			repositoryEvidence[entity.ID] = struct{}{}
		}
		if entity.Freshness != domain.FreshnessCurrent {
			stale = append(stale, entity.ID)
		}
	}
	if subjectID != "" {
		if _, exists := entities[subjectID]; !exists {
			return ImpactAnalysis{}, fmt.Errorf("%w: impact subject %s", application.ErrNotFound, subjectID)
		}
	}

	neighbors := make(map[string][]string)
	directEvidence := make(map[string]struct{})
	for _, relation := range snapshot.Relations {
		neighbors[relation.FromID] = append(neighbors[relation.FromID], relation.ToID)
		neighbors[relation.ToID] = append(neighbors[relation.ToID], relation.FromID)
		if relation.Type == domain.RelationEvidencedBy && relation.FromID == subjectID {
			if _, ok := repositoryEvidence[relation.ToID]; ok {
				directEvidence[relation.ToID] = struct{}{}
			}
		}
	}
	if subject, ok := entities[subjectID]; ok {
		for _, ref := range subject.SourceRefs {
			if _, repositoryFact := repositoryEvidence[ref]; repositoryFact {
				directEvidence[ref] = struct{}{}
			}
		}
	}

	direct := append([]string{}, neighbors[subjectID]...)
	slices.Sort(direct)
	direct = slices.Compact(direct)
	transitive := make([]string, 0)
	if subjectID != "" {
		seen := map[string]bool{subjectID: true}
		queue := append([]string{}, direct...)
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			if seen[id] {
				continue
			}
			seen[id] = true
			if !slices.Contains(direct, id) {
				transitive = append(transitive, id)
			}
			queue = append(queue, neighbors[id]...)
		}
	}
	evidenceIDs := make([]string, 0)
	if subjectID == "" {
		for id := range repositoryEvidence {
			evidenceIDs = append(evidenceIDs, id)
		}
	} else {
		for id := range directEvidence {
			evidenceIDs = append(evidenceIDs, id)
		}
	}
	slices.Sort(evidenceIDs)
	slices.Sort(transitive)
	slices.Sort(stale)
	return ImpactAnalysis{
		ProjectID: projectID, ProjectRevision: snapshot.Project.Revision, Mode: string(snapshot.Project.Mode), SubjectID: subjectID,
		RepositoryEvidenceIDs: evidenceIDs, DirectlyAffectedIDs: direct, TransitivelyAffectedIDs: transitive, PotentiallyStaleIDs: stale,
	}, nil
}
