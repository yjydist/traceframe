package artifacts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"

	"github.com/yjydist/traceframe/internal/domain"
)

func Render(projection Projection, renderer RendererType) (string, string, error) {
	switch renderer {
	case RendererHTML:
		return renderHTML(projection), "text/html; charset=utf-8", nil
	case RendererMarkdown:
		return renderMarkdown(projection), "text/markdown; charset=utf-8", nil
	case RendererJSON:
		content, err := renderJSON(projection)
		return content, "application/json", err
	case RendererMermaid:
		if !supportsMermaid(projection.Definition.ViewType) {
			return "", "", fmt.Errorf("renderer mermaid does not support %s", projection.Definition.ViewType)
		}
		return renderMermaid(projection), "text/vnd.mermaid; charset=utf-8", nil
	default:
		return "", "", fmt.Errorf("unsupported renderer %q", renderer)
	}
}

func renderHTML(projection Projection) string {
	var builder strings.Builder
	builder.WriteString("<article data-view-type=\"")
	builder.WriteString(html.EscapeString(projection.Definition.ViewType))
	builder.WriteString("\"><header><h1>")
	builder.WriteString(html.EscapeString(projection.Definition.Title))
	builder.WriteString("</h1><p>Project revision ")
	builder.WriteString(fmt.Sprint(projection.Snapshot.Project.Revision))
	builder.WriteString("</p></header>")
	for _, entity := range projection.Entities {
		builder.WriteString("<section id=\"")
		builder.WriteString(html.EscapeString(entity.ID))
		builder.WriteString("\" data-entity-id=\"")
		builder.WriteString(html.EscapeString(entity.ID))
		builder.WriteString("\"><h2>")
		builder.WriteString(html.EscapeString(entity.Title))
		builder.WriteString("</h2><p><strong>")
		builder.WriteString(html.EscapeString(string(entity.Kind)))
		builder.WriteString("</strong> · ")
		builder.WriteString(html.EscapeString(string(entity.Status)))
		builder.WriteString(" · ")
		builder.WriteString(html.EscapeString(entity.ID))
		builder.WriteString("</p><pre>")
		builder.WriteString(html.EscapeString(prettyJSON(entity.Body)))
		builder.WriteString("</pre></section>")
	}
	builder.WriteString("</article>")
	return builder.String()
}

func renderMarkdown(projection Projection) string {
	if projection.Definition.ViewType == "implementation_packet" {
		return renderImplementationPacket(projection)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\nProject revision: `%d`  \nSource checksum: `%s`\n", markdownText(projection.Definition.Title), projection.Snapshot.Project.Revision, projection.Checksum)
	for _, entity := range projection.Entities {
		fmt.Fprintf(&builder, "\n<!-- entity:%s -->\n## %s [`%s`]\n\n- Kind: `%s`\n- Status: `%s`\n- Origin: `%s`\n\n%s\n", entity.ID, markdownText(entity.Title), entity.ID, entity.Kind, entity.Status, entity.Origin, indentedJSON(entity.Body))
	}
	return builder.String()
}

func renderImplementationPacket(projection Projection) string {
	var builder strings.Builder
	project := projection.Snapshot.Project
	fmt.Fprintf(&builder, "# %s implementation packet\n\n- Project: `%s`\n- Model revision: `%d`\n- Checksum: `%s`\n", markdownText(project.Name), project.ID, project.Revision, projection.Checksum)
	if projection.Baseline != nil {
		fmt.Fprintf(&builder, "- Baseline: `%s`\n- Approved at: `%s`\n", projection.Baseline.ID, projection.Baseline.ApprovedAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	sections := []struct {
		title string
		kinds []domain.EntityKind
	}{
		{"Purpose and scope", []domain.EntityKind{domain.KindGoal, domain.KindStakeholder, domain.KindScopeItem}},
		{"Context, constraints, and assumptions", []domain.EntityKind{domain.KindContext, domain.KindConstraint, domain.KindAssumption, domain.KindTerm, domain.KindEvidence}},
		{"Scenarios and requirements", []domain.EntityKind{domain.KindScenario, domain.KindRequirement}},
		{"Quality and risk", []domain.EntityKind{domain.KindQualityScenario, domain.KindRisk, domain.KindExperiment}},
		{"Decisions", []domain.EntityKind{domain.KindOption, domain.KindDecision, domain.KindSystemElement}},
		{"Delivery and verification", []domain.EntityKind{domain.KindWorkSlice, domain.KindVerification}},
		{"Unresolved items", []domain.EntityKind{domain.KindQuestion}},
	}
	for _, section := range sections {
		entities := filterEntities(projection.Entities, section.kinds)
		if len(entities) == 0 {
			continue
		}
		fmt.Fprintf(&builder, "\n## %s\n", section.title)
		for _, entity := range entities {
			fmt.Fprintf(&builder, "\n<!-- entity:%s -->\n### %s [`%s`]\n\n%s\n", entity.ID, markdownText(entity.Title), entity.ID, indentedJSON(entity.Body))
		}
	}
	builder.WriteString("\n## Implementation instructions\n\nDo not reinterpret rejected options or silently resolve open questions, assumptions, risks, or approval requirements. Report implementation evidence against the stable entity IDs above.\n")
	return builder.String()
}

func renderJSON(projection Projection) (string, error) {
	value := struct {
		ViewType       string            `json:"view_type"`
		Title          string            `json:"title"`
		SourceRevision int64             `json:"source_revision"`
		Checksum       string            `json:"source_checksum"`
		Entities       []domain.Entity   `json:"entities"`
		Relations      []domain.Relation `json:"relations"`
	}{projection.Definition.ViewType, projection.Definition.Title, projection.Snapshot.Project.Revision, projection.Checksum, projection.Entities, projection.Relations}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

var mermaidIDPattern = regexp.MustCompile(`[^A-Za-z0-9_]`)

func renderMermaid(projection Projection) string {
	var builder strings.Builder
	builder.WriteString("flowchart TD\n")
	ids := make(map[string]struct{}, len(projection.Entities))
	for _, entity := range projection.Entities {
		id := mermaidID(entity.ID)
		ids[entity.ID] = struct{}{}
		label := mermaidLabel(entity.Title + " [" + entity.ID + "]")
		fmt.Fprintf(&builder, "  %s[\"%s\"]\n", id, label)
	}
	for _, relation := range projection.Relations {
		_, from := ids[relation.FromID]
		_, to := ids[relation.ToID]
		if from && to {
			fmt.Fprintf(&builder, "  %s -->|%s| %s\n", mermaidID(relation.FromID), relation.Type, mermaidID(relation.ToID))
		}
	}
	return builder.String()
}

func supportsMermaid(viewType string) bool {
	switch viewType {
	case "traceability_readiness", "interaction", "data_model", "runtime_interaction", "deployment", "security":
		return true
	default:
		return false
	}
}

func mermaidID(value string) string { return "n_" + mermaidIDPattern.ReplaceAllString(value, "_") }

func mermaidLabel(value string) string {
	value = strings.NewReplacer("\"", "'", "[", "(", "]", ")", "|", "/", "\n", " ", "\r", " ").Replace(value)
	return value
}

func markdownText(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func prettyJSON(value json.RawMessage) string {
	var output bytes.Buffer
	if json.Indent(&output, value, "", "  ") == nil {
		return output.String()
	}
	return string(value)
}

func indentedJSON(value json.RawMessage) string {
	return "    " + strings.ReplaceAll(prettyJSON(value), "\n", "\n    ")
}

func filterEntities(entities []domain.Entity, kinds []domain.EntityKind) []domain.Entity {
	result := make([]domain.Entity, 0)
	for _, entity := range entities {
		if slices.Contains(kinds, entity.Kind) {
			result = append(result, entity)
		}
	}
	return result
}
