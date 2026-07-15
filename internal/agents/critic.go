package agents

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	"github.com/yjydist/traceframe/internal/review"
	traceprompts "github.com/yjydist/traceframe/prompts"
)

type ReviewProposal struct {
	RunID                 string                `json:"run_id"`
	BaseRevision          int64                 `json:"base_revision"`
	Summary               string                `json:"summary"`
	Findings              []review.FindingDraft `json:"findings"`
	Warnings              []string              `json:"warnings"`
	Unresolved            []string              `json:"unresolved"`
	RecommendedNextAction string                `json:"recommended_next_action"`
}

type criticContext struct {
	RunID     string            `json:"run_id"`
	Task      string            `json:"task"`
	Project   domain.Project    `json:"project"`
	Entities  []domain.Entity   `json:"entities"`
	Relations []domain.Relation `json:"relations"`
	Gate      string            `json:"gate"`
}

func BuildCriticRequest(run domain.AgentRun, snapshot domain.Snapshot) (models.GenerateRequest, error) {
	system, responseSchema, err := traceprompts.CriticV1()
	if err != nil {
		return models.GenerateRequest{}, fmt.Errorf("load critic prompt: %w", err)
	}
	selected := selectedSnapshot(domain.RoleCritic, snapshot)
	contextJSON, err := json.Marshal(criticContext{RunID: run.ID, Task: run.Task, Project: snapshot.Project, Entities: selected.Entities, Relations: selected.Relations, Gate: "Find blocking contradictions, missing exact approvals, untreated residual risk, missing traceability or verification, and weak evidence before READY."})
	if err != nil {
		return models.GenerateRequest{}, fmt.Errorf("encode critic context: %w", err)
	}
	return models.GenerateRequest{
		Messages: []models.Message{{Role: "system", Content: system}, {Role: "user", Content: string(contextJSON)}},
		Tools:    []models.ToolSchema{}, ResponseSchema: responseSchema, TemperaturePolicy: models.TemperatureDeterministic,
		TokenBudget: models.TokenBudget{MaxInputTokens: run.Budget.MaxInputTokens, MaxOutputTokens: run.Budget.MaxOutputTokens},
		Metadata:    map[string]string{"run_id": run.ID, "project_id": run.ProjectID, "role": string(run.Role), "prompt_version": run.PromptVersion},
	}, nil
}

func ValidateReviewProposal(proposal ReviewProposal) error {
	if strings.TrimSpace(proposal.RunID) == "" || proposal.BaseRevision < 1 || strings.TrimSpace(proposal.Summary) == "" {
		return fmt.Errorf("%w: run_id, base_revision, and summary are required", domain.ErrInvalid)
	}
	if len(proposal.Findings) == 0 || len(proposal.Findings) > 100 {
		return fmt.Errorf("%w: review proposal must contain between 1 and 100 findings", domain.ErrInvalid)
	}
	for index, finding := range proposal.Findings {
		if strings.TrimSpace(finding.ID) == "" || strings.TrimSpace(finding.Claim) == "" || strings.TrimSpace(finding.Evidence) == "" || strings.TrimSpace(finding.RecommendedResolution) == "" {
			return fmt.Errorf("%w: finding %d is incomplete", domain.ErrInvalid, index)
		}
	}
	return nil
}
