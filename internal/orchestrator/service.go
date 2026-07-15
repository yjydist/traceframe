package orchestrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yjydist/traceframe/internal/agents"
	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/domain"
	"github.com/yjydist/traceframe/internal/jobs"
	"github.com/yjydist/traceframe/internal/models"
)

type RunRequest struct {
	Role           domain.AgentRole  `json:"role"`
	Task           string            `json:"task"`
	IdempotencyKey string            `json:"idempotency_key"`
	Budget         *domain.RunBudget `json:"budget,omitempty"`
}

type Service struct {
	projects *application.ProjectService
	store    Store
	jobs     jobs.Store
	model    models.ModelClient
	logger   *slog.Logger
	now      func() time.Time
	workerID string
	lease    time.Duration
	poll     time.Duration

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func NewService(projects *application.ProjectService, store Store, jobStore jobs.Store, model models.ModelClient, logger *slog.Logger) *Service {
	return &Service{projects: projects, store: store, jobs: jobStore, model: model, logger: logger, now: time.Now, workerID: domain.NewID("worker"), lease: 15 * time.Second, poll: 100 * time.Millisecond, active: make(map[string]context.CancelFunc)}
}

func (s *Service) CreateRun(ctx context.Context, projectID string, request RunRequest) (domain.AgentRun, bool, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	request.Task = strings.TrimSpace(request.Task)
	if request.Role == "" {
		request.Role = domain.RoleDiscovery
	}
	if request.Role != domain.RoleDiscovery {
		return domain.AgentRun{}, false, fmt.Errorf("%w: only discovery runs are enabled", domain.ErrInvalid)
	}
	if request.Task == "" || len(request.Task) > 500 || strings.EqualFold(request.Task, "improve the design") {
		return domain.AgentRun{}, false, fmt.Errorf("%w: task must be specific, bounded, and at most 500 characters", domain.ErrInvalid)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return domain.AgentRun{}, false, fmt.Errorf("%w: idempotency_key is required", domain.ErrInvalid)
	}
	budget := domain.DefaultRunBudget()
	if request.Budget != nil {
		budget = *request.Budget
	}
	requestBytes, _ := json.Marshal(request)
	digest := sha256.Sum256(requestBytes)
	now := s.now().UTC()
	contextIDs := make([]string, 0, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		contextIDs = append(contextIDs, entity.ID)
	}
	run := domain.AgentRun{
		ID: domain.NewID("run"), ProjectID: projectID, Role: request.Role, State: domain.RunQueued, Task: request.Task,
		BaseRevision: snapshot.Project.Revision, Budget: budget, IdempotencyKey: request.IdempotencyKey, RequestChecksum: hex.EncodeToString(digest[:]),
		PromptVersion: "discovery.v1", ResponseSchemaVersion: "proposal.v1", SelectedContextIDs: contextIDs, AllowedTools: []string{}, CreatedAt: now, UpdatedAt: now,
	}
	job := jobs.Job{ID: domain.NewID("job"), ProjectID: projectID, RunID: run.ID, Type: "agent_run", State: jobs.Queued, MaxAttempts: budget.MaxAttempts, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	return s.store.CreateRun(ctx, run, job)
}

func (s *Service) GetRun(ctx context.Context, projectID, runID string) (domain.AgentRun, error) {
	return s.store.GetRun(ctx, projectID, runID)
}

func (s *Service) ListRuns(ctx context.Context, projectID string, limit int) ([]domain.AgentRun, error) {
	return s.store.ListRuns(ctx, projectID, limit)
}

func (s *Service) Cancel(ctx context.Context, projectID, runID string) (domain.AgentRun, error) {
	run, err := s.store.RequestCancellation(ctx, projectID, runID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	s.mu.Lock()
	cancel := s.active[runID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return run, nil
}

func (s *Service) Start(ctx context.Context) {
	go s.worker(ctx)
}

func (s *Service) RunOnce(ctx context.Context) (bool, error) {
	job, err := s.jobs.Claim(ctx, s.workerID, s.now().UTC(), s.lease)
	if err != nil || job == nil {
		return false, err
	}
	return true, s.execute(ctx, *job)
}

func (s *Service) worker(ctx context.Context) {
	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()
	for {
		processed, err := s.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Error("agent job failed", "error", err)
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) execute(parent context.Context, job jobs.Job) error {
	run, err := s.store.GetRun(parent, job.ProjectID, job.RunID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, run.Budget.WallClockTimeout)
	s.register(run.ID, cancel)
	renewDone := make(chan struct{})
	go s.renewLease(ctx, cancel, job, renewDone)
	defer func() { cancel(); <-renewDone; s.unregister(run.ID) }()

	fail := func(code string, cause error) error {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if parent.Err() != nil {
			return parent.Err()
		}
		requested, _ := s.store.CancellationRequested(cleanup, run.ID)
		if requested {
			_, _ = s.store.TransitionRun(cleanup, run.ProjectID, run.ID, domain.RunCancelled, "cancelled", "run cancelled by user")
			_ = s.jobs.CancelRun(cleanup, run.ID, s.now().UTC())
			return nil
		}
		_, _ = s.store.TransitionRun(cleanup, run.ProjectID, run.ID, domain.RunFailed, code, cause.Error())
		_ = s.jobs.FailTerminal(cleanup, job.ID, s.workerID, s.now().UTC(), cause)
		return cause
	}

	if _, err = s.store.TransitionRun(ctx, run.ProjectID, run.ID, domain.RunPreparingContext, "", ""); err != nil {
		return fail("transition_failed", err)
	}
	snapshot, err := s.projects.Snapshot(ctx, run.ProjectID)
	if err != nil {
		return fail("context_failed", err)
	}
	if snapshot.Project.Revision != run.BaseRevision {
		return fail("revision_conflict", fmt.Errorf("%w: run revision %d, current revision %d", application.ErrConflict, run.BaseRevision, snapshot.Project.Revision))
	}
	request, err := agents.BuildDiscoveryRequest(run, snapshot)
	if err != nil {
		return fail("context_failed", err)
	}
	if _, err = s.store.TransitionRun(ctx, run.ProjectID, run.ID, domain.RunWaitingForModel, "", ""); err != nil {
		return fail("transition_failed", err)
	}
	proposal, response, usage, err := s.generateProposal(ctx, run, request)
	if usage.ModelTurns > 0 {
		recordContext := ctx
		var recordCancel context.CancelFunc
		if ctx.Err() != nil {
			recordContext, recordCancel = context.WithTimeout(context.Background(), 5*time.Second)
			defer recordCancel()
		}
		if recordErr := s.store.RecordModelResult(recordContext, run.ProjectID, run.ID, response, usage); recordErr != nil {
			return fail("persistence_failed", recordErr)
		}
	}
	if err != nil {
		return fail("model_failed", err)
	}
	if _, err = s.store.TransitionRun(ctx, run.ProjectID, run.ID, domain.RunValidating, "", ""); err != nil {
		return fail("transition_failed", err)
	}
	if proposal.RunID != run.ID || proposal.BaseRevision != run.BaseRevision {
		return fail("proposal_mismatch", fmt.Errorf("%w: proposal run or revision mismatch", domain.ErrInvalid))
	}
	if err := agents.ValidateProposal(run.Role, proposal); err != nil {
		return fail("proposal_invalid", err)
	}
	agents.NormalizeProposal(&proposal)
	requested, err := s.store.CancellationRequested(ctx, run.ID)
	if err != nil || requested || ctx.Err() != nil {
		if err == nil {
			err = context.Canceled
		}
		return fail("cancelled", err)
	}
	proposalBytes, _ := json.Marshal(proposal)
	proposalDigest := sha256.Sum256(proposalBytes)
	checksum := hex.EncodeToString(proposalDigest[:])
	_, err = s.projects.ApplyCommands(ctx, run.ProjectID, application.CommandEnvelope{ExpectedRevision: run.BaseRevision, Actor: "agent:" + string(run.Role), Commands: proposal.Commands})
	if err != nil {
		return fail("proposal_rejected", err)
	}
	if err := s.store.RecordProposalOutcome(ctx, run.ProjectID, run.ID, checksum, "applied"); err != nil {
		return fail("persistence_failed", err)
	}
	if _, err = s.store.TransitionRun(ctx, run.ProjectID, run.ID, domain.RunCompleted, "", ""); err != nil {
		return fail("transition_failed", err)
	}
	if err := s.jobs.Complete(ctx, job.ID, s.workerID, s.now().UTC()); err != nil {
		return err
	}
	return nil
}

func (s *Service) renewLease(ctx context.Context, cancel context.CancelFunc, job jobs.Job, done chan<- struct{}) {
	defer close(done)
	interval := s.lease / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.jobs.Renew(ctx, job.ID, s.workerID, s.now().UTC(), s.lease); err != nil {
				s.logger.Error("job lease renewal failed", "job_id", job.ID, "run_id", job.RunID, "error", err)
				cancel()
				return
			}
		}
	}
}

func (s *Service) generateProposal(ctx context.Context, run domain.AgentRun, request models.GenerateRequest) (agents.Proposal, models.GenerateResponse, domain.RunUsage, error) {
	usage := domain.RunUsage{}
	var lastResponse models.GenerateResponse
	repairAttempted := false
	for turn := 1; turn <= run.Budget.MaxModelTurns; turn++ {
		response, err := s.model.Generate(ctx, request)
		usage.ModelTurns++
		if budgetErr := run.Budget.Check(usage); budgetErr != nil {
			return agents.Proposal{}, response, usage, budgetErr
		}
		if err != nil {
			var providerError *models.ProviderError
			if errors.As(err, &providerError) && providerError.Transient && turn < run.Budget.MaxModelTurns {
				timer := time.NewTimer(time.Duration(turn) * 100 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return agents.Proposal{}, response, usage, ctx.Err()
				case <-timer.C:
					continue
				}
			}
			return agents.Proposal{}, response, usage, err
		}
		lastResponse = response
		usage.InputTokens += response.Usage.InputTokens
		usage.OutputTokens += response.Usage.OutputTokens
		if err := run.Budget.Check(usage); err != nil {
			return agents.Proposal{}, response, usage, err
		}
		proposal, err := decodeProposal(response.Output)
		if err == nil {
			return proposal, response, usage, nil
		}
		if repairAttempted || turn == run.Budget.MaxModelTurns {
			return agents.Proposal{}, response, usage, fmt.Errorf("structured proposal invalid after repair: %w", err)
		}
		repairAttempted = true
		request.Messages = append(request.Messages,
			models.Message{Role: "assistant", Content: string(response.Output)},
			models.Message{Role: "user", Content: "The response did not match the required JSON schema. Return one corrected JSON object only."},
		)
	}
	return agents.Proposal{}, lastResponse, usage, fmt.Errorf("no model turn available")
}

func decodeProposal(data []byte) (agents.Proposal, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var proposal agents.Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return agents.Proposal{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return agents.Proposal{}, fmt.Errorf("proposal contains trailing JSON")
	}
	return proposal, nil
}

func (s *Service) register(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[runID] = cancel
}

func (s *Service) unregister(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, runID)
}
