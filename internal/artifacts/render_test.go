package artifacts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/review"
)

func TestRenderersAreDeterministicSafeAndPreserveStableIDs(t *testing.T) {
	approved := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	projection := Projection{
		Definition: Definition{ViewType: "implementation_packet", Title: "Implementation packet", Kinds: allKinds()},
		Snapshot:   domain.Snapshot{Project: domain.Project{ID: "project_1", Name: "<script>project</script>", Revision: 7}, Entities: []domain.Entity{{ID: "goal_stable", Kind: domain.KindGoal, Title: "<script>goal</script>", Body: json.RawMessage(`{"outcome":"Safe output","success_signals":["Visible"],"priority":"must"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Freshness: domain.FreshnessCurrent}}},
		Entities:   []domain.Entity{{ID: "goal_stable", Kind: domain.KindGoal, Title: "<script>goal</script>", Body: json.RawMessage(`{"outcome":"Safe output","success_signals":["Visible"],"priority":"must"}`), Status: domain.EntityConfirmed, Origin: domain.OriginUser, Freshness: domain.FreshnessCurrent}},
		Baseline:   &review.Baseline{ID: "baseline_1", ApprovedAt: approved}, Checksum: "checksum_1",
	}
	first, _, err := Render(projection, RendererMarkdown)
	second, _, _ := Render(projection, RendererMarkdown)
	if err != nil || first != second || !strings.Contains(first, "entity:goal_stable") || !strings.Contains(first, "`goal_stable`") || strings.Contains(first, "<script>") {
		t.Fatalf("markdown rendering is not deterministic/safe/stable:\n%s\nerror=%v", first, err)
	}
	if strings.Contains(first, "## Unresolved items") || strings.Contains(first, "## Quality and risk") {
		t.Fatalf("empty implementation sections were rendered:\n%s", first)
	}
	htmlContent, _, _ := Render(projection, RendererHTML)
	if strings.Contains(htmlContent, "<script>") || !strings.Contains(htmlContent, "data-entity-id=\"goal_stable\"") {
		t.Fatalf("unsafe HTML output: %s", htmlContent)
	}
	jsonContent, _, err := Render(projection, RendererJSON)
	if err != nil || !json.Valid([]byte(jsonContent)) {
		t.Fatalf("JSON output = %q, %v", jsonContent, err)
	}
}
