package fake

import (
	"context"
	"errors"
	"sync"

	"github.com/yjydist/traceframe/internal/models"
)

type Result struct {
	Response models.GenerateResponse
	Events   []models.StreamEvent
	Err      error
}

type Client struct {
	mu       sync.Mutex
	results  []Result
	requests []models.GenerateRequest
}

func New(results ...Result) *Client {
	return &Client{results: append([]Result{}, results...)}
}

func (c *Client) Push(results ...Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, results...)
}

func (c *Client) Generate(ctx context.Context, req models.GenerateRequest) (models.GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.GenerateResponse{}, err
	}
	result, err := c.next(req)
	if err != nil {
		return models.GenerateResponse{}, err
	}
	return result.Response, result.Err
}

func (c *Client) Stream(ctx context.Context, req models.GenerateRequest) (<-chan models.StreamEvent, error) {
	result, err := c.next(req)
	if err != nil {
		return nil, err
	}
	if result.Err != nil {
		return nil, result.Err
	}
	events := make(chan models.StreamEvent)
	go func() {
		defer close(events)
		for _, event := range result.Events {
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

func (c *Client) Requests() []models.GenerateRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]models.GenerateRequest{}, c.requests...)
}

func (c *Client) next(req models.GenerateRequest) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if len(c.results) == 0 {
		return Result{}, errors.New("fake model has no queued result")
	}
	result := c.results[0]
	c.results = c.results[1:]
	return result, nil
}
