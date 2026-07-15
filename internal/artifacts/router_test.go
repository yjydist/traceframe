package artifacts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/workflow"
)

func TestEvaluationFixturesRouteOnlyTriggeredConditionalViews(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "evals", "v1", "fixtures.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures []struct {
		ID               string   `json:"id"`
		ExpectedConcerns []string `json:"expected_concerns"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	viewForConcern := map[string]string{"interaction": "interaction", "api": "interface_contract", "data": "data_model", "migration": "migration_strategy", "runtime": "runtime_interaction", "deployment": "deployment", "security": "security", "privacy": "privacy", "reliability": "failure_recovery", "performance": "performance", "compatibility": "compatibility", "repository_change": "current_state_impact", "experiment": "experiment"}
	snapshot := domain.Snapshot{Entities: []domain.Entity{}}
	for index, kind := range allKinds() {
		snapshot.Entities = append(snapshot.Entities, domain.Entity{ID: "entity_" + string(rune('a'+index)), Kind: kind, Status: domain.EntityConfirmed, Freshness: domain.FreshnessCurrent})
	}
	for _, fixture := range fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			concerns := make([]workflow.RoutedConcern, len(fixture.ExpectedConcerns))
			for index, name := range fixture.ExpectedConcerns {
				concerns[index] = workflow.RoutedConcern{Name: name, Mandatory: name == "security" || name == "privacy" || name == "migration" || name == "compatibility"}
			}
			definitions := Route(snapshot, concerns)
			views := make([]string, len(definitions))
			for index, definition := range definitions {
				views[index] = definition.ViewType
			}
			for concern, view := range viewForConcern {
				want := slices.Contains(fixture.ExpectedConcerns, concern)
				got := slices.Contains(views, view)
				if got != want {
					t.Fatalf("views %v applicability for %s = %v, want %v", views, concern, got, want)
				}
			}
		})
	}
}
