package models

import (
	"context"
	"encoding/json"
	"fmt"
)

type ProviderError struct {
	Code      string
	Message   string
	Transient bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("model provider %s: %s", e.Code, e.Message)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type TemperaturePolicy string

const (
	TemperatureDeterministic TemperaturePolicy = "deterministic"
	TemperatureBalanced      TemperaturePolicy = "balanced"
	TemperatureCreative      TemperaturePolicy = "creative"
)

type TokenBudget struct {
	MaxInputTokens  int `json:"max_input_tokens"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

type GenerateRequest struct {
	Messages          []Message         `json:"messages"`
	Tools             []ToolSchema      `json:"tools"`
	ResponseSchema    json.RawMessage   `json:"response_schema"`
	TemperaturePolicy TemperaturePolicy `json:"temperature_policy"`
	TokenBudget       TokenBudget       `json:"token_budget"`
	Metadata          map[string]string `json:"metadata"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type GenerateResponse struct {
	Output            json.RawMessage `json:"output"`
	ModelIdentifier   string          `json:"model_identifier"`
	ProviderRequestID string          `json:"provider_request_id,omitempty"`
	FinishReason      string          `json:"finish_reason"`
	Usage             Usage           `json:"usage"`
}

type StreamEventType string

const (
	StreamTextDelta StreamEventType = "text_delta"
	StreamToolCall  StreamEventType = "tool_call"
	StreamCompleted StreamEventType = "completed"
	StreamError     StreamEventType = "error"
)

type StreamEvent struct {
	Type  StreamEventType `json:"type"`
	Delta string          `json:"delta,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type ModelClient interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
	Stream(ctx context.Context, req GenerateRequest) (<-chan StreamEvent, error)
}
