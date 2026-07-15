package agents

import (
	"encoding/json"
	"fmt"

	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/models"
	traceprompts "github.com/yjydist/traceframe/prompts"
)

type discoveryContext struct {
	RunID        string            `json:"run_id"`
	Task         string            `json:"task"`
	Project      domain.Project    `json:"project"`
	Entities     []domain.Entity   `json:"entities"`
	Relations    []domain.Relation `json:"relations"`
	Gate         string            `json:"gate"`
	QuestionRule string            `json:"question_rule"`
}

func BuildDiscoveryRequest(run domain.AgentRun, snapshot domain.Snapshot) (models.GenerateRequest, error) {
	system, responseSchema, err := traceprompts.DiscoveryV1()
	if err != nil {
		return models.GenerateRequest{}, fmt.Errorf("load discovery prompt: %w", err)
	}
	contextPack := discoveryContext{
		RunID: run.ID, Task: run.Task, Project: snapshot.Project, Entities: snapshot.Entities, Relations: snapshot.Relations,
		Gate:         "INTAKE requires recorded classification; FRAMING requires a confirmed goal, clear boundary, and explicit non-goals; CONTEXT requires material constraints and dependencies with visible evidence gaps.",
		QuestionRule: "Prioritize impact * uncertainty * irreversibility and present at most three related questions per batch.",
	}
	contextJSON, err := json.Marshal(contextPack)
	if err != nil {
		return models.GenerateRequest{}, fmt.Errorf("encode discovery context: %w", err)
	}
	return models.GenerateRequest{
		Messages: []models.Message{{Role: "system", Content: system}, {Role: "user", Content: string(contextJSON)}},
		Tools:    []models.ToolSchema{}, ResponseSchema: responseSchema, TemperaturePolicy: models.TemperatureDeterministic,
		TokenBudget: models.TokenBudget{MaxInputTokens: run.Budget.MaxInputTokens, MaxOutputTokens: run.Budget.MaxOutputTokens},
		Metadata:    map[string]string{"run_id": run.ID, "project_id": run.ProjectID, "role": string(run.Role), "prompt_version": run.PromptVersion},
	}, nil
}
