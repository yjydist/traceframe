package models

import "context"

type UnconfiguredClient struct{}

func (UnconfiguredClient) Generate(context.Context, GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, &ProviderError{Code: "provider_not_configured", Message: "configure a model provider before starting agent runs", Transient: false}
}

func (UnconfiguredClient) Stream(context.Context, GenerateRequest) (<-chan StreamEvent, error) {
	return nil, &ProviderError{Code: "provider_not_configured", Message: "configure a model provider before starting agent runs", Transient: false}
}
