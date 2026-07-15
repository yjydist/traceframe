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
	"github.com/yjydist/traceframe/internal/review"
)

type RunRequest struct {
	Role           domain.AgentRole  `json:"role"`
	Task           string            `json:"task"`
	IdempotencyKey string            `json:"idempotency_key"`
	Budget         *domain.RunBudget `json:"budget,omitempty"`
}

type Service struct {
	projects  *application.ProjectService
	store     Store
	jobs      jobs.Store
	model     models.ModelClient
	approvals interface {
		EnsureRequiredApprovals(context.Context, string, string) error
	}
	reviews interface {
		SubmitFindings(context.Context, string, string, int64, []review.FindingDraft) error
	}
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

func (s *Service) SetApprovalRequester(requester interface {
	EnsureRequiredApprovals(context.Context, string, string) error
}) {
	s.approvals = requester
}

func (s *Service) SetReviewSubmitter(submitter interface {
	SubmitFindings(context.Context, string, string, int64, []review.FindingDraft) error
}) {
	s.reviews = submitter
}

func (s *Service) CreateRun(ctx context.Context, projectID string, request RunRequest) (domain.AgentRun, bool, error) {
	snapshot, err := s.projects.Snapshot(ctx, projectID)
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	request.Task = strings.TrimSpace(request.Task)
	if request.Role == "" {
		request.Role = defaultRole(snapshot.Project.Stage)
	}
	if !roleAllowedAtStage(request.Role, snapshot.Project.Stage) {
		return domain.AgentRun{}, false, fmt.Errorf("%w: %s runs are not allowed during %s", domain.ErrInvalid, request.Role, snapshot.Project.Stage)
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
	contextIDs := agents.SelectContextIDs(request.Role, snapshot)
	promptVersion := string(request.Role) + ".v1"
	run := domain.AgentRun{
		ID: domain.NewID("run"), ProjectID: projectID, Role: request.Role, State: domain.RunQueued, Task: request.Task,
		BaseRevision: snapshot.Project.Revision, Budget: budget, IdempotencyKey: request.IdempotencyKey, RequestChecksum: hex.EncodeToString(digest[:]),
		PromptVersion: promptVersion, ResponseSchemaVersion: "proposal.v1", SelectedContextIDs: contextIDs, AllowedTools: []string{}, CreatedAt: now, UpdatedAt: now,
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
		_ = s.store.RecordProposalOutcome(ctx, run.ProjectID, run.ID, "", "reconciliation_required")
		return fail("reconciliation_required", fmt.Errorf("%w: run revision %d, current revision %d; rebuild context and reconcile", application.ErrConflict, run.BaseRevision, snapshot.Project.Revision))
	}
	request, err := agents.BuildProposalRequest(run, snapshot)
	if err != nil {
		return fail("context_failed", err)
	}
	if _, err = s.store.TransitionRun(ctx, run.ProjectID, run.ID, domain.RunWaitingForModel, "", ""); err != nil {
		return fail("transition_failed", err)
	}
	var proposal agents.Proposal
	var reviewProposal agents.ReviewProposal
	var response models.GenerateResponse
	var usage domain.RunUsage
	if run.Role == domain.RoleCritic {
		reviewProposal, response, usage, err = s.generateReviewProposal(ctx, run, request)
	} else {
		proposal, response, usage, err = s.generateProposal(ctx, run, request)
	}
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
	if run.Role == domain.RoleCritic {
		if reviewProposal.RunID != run.ID || reviewProposal.BaseRevision != run.BaseRevision {
			return fail("proposal_mismatch", fmt.Errorf("%w: review proposal run or revision mismatch", domain.ErrInvalid))
		}
		if err := agents.ValidateReviewProposal(reviewProposal); err != nil {
			return fail("proposal_invalid", err)
		}
		requested, cancelErr := s.store.CancellationRequested(ctx, run.ID)
		if cancelErr != nil || requested || ctx.Err() != nil {
			if cancelErr == nil {
				cancelErr = context.Canceled
			}
			return fail("cancelled", cancelErr)
		}
		if s.reviews == nil {
			return fail("review_store_unavailable", fmt.Errorf("review submission is unavailable"))
		}
		proposalBytes, _ := json.Marshal(reviewProposal)
		proposalDigest := sha256.Sum256(proposalBytes)
		checksum := hex.EncodeToString(proposalDigest[:])
		if err := s.reviews.SubmitFindings(ctx, run.ProjectID, run.ID, run.BaseRevision, reviewProposal.Findings); err != nil {
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
		if errors.Is(err, application.ErrConflict) {
			_ = s.store.RecordProposalOutcome(ctx, run.ProjectID, run.ID, checksum, "reconciliation_required")
			return fail("reconciliation_required", fmt.Errorf("%w: proposal conflicts with a newer project revision; reconcile explicitly", err))
		}
		return fail("proposal_rejected", err)
	}
	if s.approvals != nil {
		if err := s.approvals.EnsureRequiredApprovals(ctx, run.ProjectID, "agent:"+string(run.Role)); err != nil {
			return fail("approval_request_failed", err)
		}
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

func (s *Service) generateReviewProposal(ctx context.Context, run domain.AgentRun, request models.GenerateRequest) (agents.ReviewProposal, models.GenerateResponse, domain.RunUsage, error) {
	usage := domain.RunUsage{}
	var lastResponse models.GenerateResponse
	repairAttempted := false
	for turn := 1; turn <= run.Budget.MaxModelTurns; turn++ {
		response, err := s.model.Generate(ctx, request)
		usage.ModelTurns++
		if budgetErr := run.Budget.Check(usage); budgetErr != nil {
			return agents.ReviewProposal{}, response, usage, budgetErr
		}
		if err != nil {
			var providerError *models.ProviderError
			if errors.As(err, &providerError) && providerError.Transient && turn < run.Budget.MaxModelTurns {
				timer := time.NewTimer(time.Duration(turn) * 100 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return agents.ReviewProposal{}, response, usage, ctx.Err()
				case <-timer.C:
					continue
				}
			}
			return agents.ReviewProposal{}, response, usage, err
		}
		lastResponse = response
		usage.InputTokens += response.Usage.InputTokens
		usage.OutputTokens += response.Usage.OutputTokens
		if err := run.Budget.Check(usage); err != nil {
			return agents.ReviewProposal{}, response, usage, err
		}
		proposal, err := decodeReviewProposal(response.Output)
		if err == nil {
			return proposal, response, usage, nil
		}
		if repairAttempted || turn == run.Budget.MaxModelTurns {
			return agents.ReviewProposal{}, response, usage, fmt.Errorf("structured review proposal invalid after repair: %w", err)
		}
		repairAttempted = true
		request.Messages = append(request.Messages, models.Message{Role: "assistant", Content: string(response.Output)}, models.Message{Role: "user", Content: "The response did not match the required JSON schema. Return one corrected JSON object only."})
	}
	return agents.ReviewProposal{}, lastResponse, usage, fmt.Errorf("no model turn available")
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

func decodeReviewProposal(data []byte) (agents.ReviewProposal, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var proposal agents.ReviewProposal
	if err := decoder.Decode(&proposal); err != nil {
		return agents.ReviewProposal{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return agents.ReviewProposal{}, fmt.Errorf("review proposal contains trailing JSON")
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

func defaultRole(stage domain.ProjectStage) domain.AgentRole {
	switch stage {
	case domain.StageScenarios, domain.StageRequirements:
		return domain.RoleRequirements
	case domain.StageShaping, domain.StageDecisions:
		return domain.RoleArchitecture
	case domain.StageDelivery:
		return domain.RoleDelivery
	case domain.StageReview:
		return domain.RoleCritic
	default:
		return domain.RoleDiscovery
	}
}

func roleAllowedAtStage(role domain.AgentRole, stage domain.ProjectStage) bool {
	switch role {
	case domain.RoleDiscovery:
		return stage == domain.StageIntake || stage == domain.StageFraming || stage == domain.StageContext
	case domain.RoleRequirements:
		return stage == domain.StageScenarios || stage == domain.StageRequirements
	case domain.RoleArchitecture:
		return stage == domain.StageShaping || stage == domain.StageDecisions
	case domain.RoleQualityRisk:
		switch stage {
		case domain.StageContext, domain.StageScenarios, domain.StageRequirements, domain.StageShaping, domain.StageDecisions:
			return true
		default:
			return false
		}
	case domain.RoleDelivery:
		return stage == domain.StageDelivery
	case domain.RoleCritic:
		return stage == domain.StageReview
	default:
		return false
	}
}
