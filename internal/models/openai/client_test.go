package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yjydist/traceframe/internal/models"
)

func TestGenerateUsesResponsesStructuredOutput(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("request = %s, authorization = %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","model":"gpt-5.6","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"run_id\":\"run_1\"}"}]}],"usage":{"input_tokens":20,"output_tokens":8}}`))
	}))
	defer server.Close()
	client := New("test-key", "gpt-5.6", server.URL+"/v1", server.Client())
	response, err := client.Generate(context.Background(), models.GenerateRequest{Messages: []models.Message{{Role: "user", Content: "test"}}, ResponseSchema: json.RawMessage(`{"type":"object"}`), TokenBudget: models.TokenBudget{MaxOutputTokens: 100}, Metadata: map[string]string{"run_id": "run_1"}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.ProviderRequestID != "resp_1" || response.Usage.InputTokens != 20 || string(response.Output) != `{"run_id":"run_1"}` {
		t.Fatalf("response = %#v", response)
	}
	if captured["store"] != false || captured["max_output_tokens"].(float64) != 100 {
		t.Fatalf("request body = %#v", captured)
	}
	text := captured["text"].(map[string]any)
	format := text["format"].(map[string]any)
	if format["type"] != "json_schema" || format["strict"] != true {
		t.Fatalf("response format = %#v", format)
	}
}

func TestGenerateClassifiesRateLimitWithoutLeakingKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"rate_limit","message":"try later"}}`))
	}))
	defer server.Close()
	client := New("sensitive-key", "gpt-5.6", server.URL, server.Client())
	_, err := client.Generate(context.Background(), models.GenerateRequest{ResponseSchema: json.RawMessage(`{}`), TokenBudget: models.TokenBudget{MaxOutputTokens: 1}})
	providerError, ok := err.(*models.ProviderError)
	if !ok || !providerError.Transient || providerError.Code != "rate_limit" {
		t.Fatalf("provider error = %#v", err)
	}
}
