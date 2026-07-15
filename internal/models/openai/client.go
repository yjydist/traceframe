package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yjydist/traceframe/internal/models"
)

type Client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func New(apiKey, model, baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Minute}
	}
	return &Client{apiKey: apiKey, model: model, baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

type responseFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type requestBody struct {
	Model           string            `json:"model"`
	Input           []models.Message  `json:"input"`
	Text            map[string]any    `json:"text"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Store           bool              `json:"store"`
}

type apiResponse struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Generate(ctx context.Context, request models.GenerateRequest) (models.GenerateResponse, error) {
	body := requestBody{
		Model: c.model, Input: request.Messages, MaxOutputTokens: request.TokenBudget.MaxOutputTokens,
		Metadata: request.Metadata, Store: false,
		Text: map[string]any{"format": responseFormat{Type: "json_schema", Name: "traceframe_proposal", Schema: request.ResponseSchema, Strict: true}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return models.GenerateResponse{}, fmt.Errorf("encode OpenAI request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(data))
	if err != nil {
		return models.GenerateResponse{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return models.GenerateResponse{}, &models.ProviderError{Code: "transport_error", Message: err.Error(), Transient: true}
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 8<<20)
	var decoded apiResponse
	if err := json.NewDecoder(limited).Decode(&decoded); err != nil {
		return models.GenerateResponse{}, &models.ProviderError{Code: "invalid_response", Message: "provider returned invalid JSON", Transient: response.StatusCode >= 500}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || decoded.Error != nil {
		code, message := fmt.Sprintf("http_%d", response.StatusCode), "provider request failed"
		if decoded.Error != nil {
			if decoded.Error.Code != "" {
				code = decoded.Error.Code
			}
			if decoded.Error.Message != "" {
				message = decoded.Error.Message
			}
		}
		return models.GenerateResponse{}, &models.ProviderError{Code: code, Message: message, Transient: response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500}
	}
	var output strings.Builder
	for _, item := range decoded.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				output.WriteString(content.Text)
			}
		}
	}
	if output.Len() == 0 {
		return models.GenerateResponse{}, &models.ProviderError{Code: "missing_output", Message: "provider response contained no output_text", Transient: false}
	}
	return models.GenerateResponse{Output: json.RawMessage(output.String()), ModelIdentifier: decoded.Model, ProviderRequestID: decoded.ID, FinishReason: decoded.Status, Usage: models.Usage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens}}, nil
}

func (c *Client) Stream(ctx context.Context, request models.GenerateRequest) (<-chan models.StreamEvent, error) {
	events := make(chan models.StreamEvent, 1)
	go func() {
		defer close(events)
		response, err := c.Generate(ctx, request)
		if err != nil {
			events <- models.StreamEvent{Type: models.StreamError, Error: err.Error()}
			return
		}
		events <- models.StreamEvent{Type: models.StreamCompleted, Data: response.Output}
	}()
	return events, nil
}
