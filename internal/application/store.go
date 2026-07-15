package application

import (
	"context"
	"errors"
	"time"

	"github.com/yjydist/traceframe/internal/domain"
)

var (
	ErrNotFound = errors.New("resource not found")
	ErrConflict = errors.New("revision conflict")
)

type EventDraft struct {
	Type    string
	Payload any
}

type Mutation func(snapshot *domain.Snapshot) ([]EventDraft, error)

type ProjectStore interface {
	CreateProject(ctx context.Context, snapshot domain.Snapshot, actor string) (domain.Snapshot, error)
	DeleteProject(ctx context.Context, projectID string, expectedRevision int64) error
	ListProjects(ctx context.Context, includeArchived bool) ([]domain.Project, error)
	GetSnapshot(ctx context.Context, projectID string) (domain.Snapshot, error)
	ListRevisions(ctx context.Context, projectID string) ([]domain.ProjectRevision, error)
	Transact(ctx context.Context, projectID string, expectedRevision int64, actor string, mutate Mutation) (domain.Snapshot, error)
	ListEvents(ctx context.Context, projectID string, afterSequence int64, limit int) ([]domain.Event, error)
	Now() time.Time
}
