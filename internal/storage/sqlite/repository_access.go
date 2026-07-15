package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/repository"
)

type RepositoryAccessStore struct {
	db *sql.DB
}

func NewRepositoryAccessStore(db *sql.DB) *RepositoryAccessStore {
	return &RepositoryAccessStore{db: db}
}

func (s *RepositoryAccessStore) CreateGrant(ctx context.Context, grant repository.Grant) (repository.Grant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return repository.Grant{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO repository_grants (id, project_id, root_path, canonical_root, created_at) VALUES (?, ?, ?, ?, ?)`,
		grant.ID, grant.ProjectID, grant.RootPath, grant.CanonicalRoot, formatTime(grant.CreatedAt))
	if err != nil {
		if isConstraintError(err) {
			return repository.Grant{}, fmt.Errorf("%w: repository root already has an active grant", application.ErrConflict)
		}
		return repository.Grant{}, fmt.Errorf("create repository grant: %w", err)
	}
	if err := insertEvent(ctx, tx, grant.ProjectID, "repository.granted", map[string]any{"grant_id": grant.ID}, grant.CreatedAt); err != nil {
		return repository.Grant{}, err
	}
	if err := tx.Commit(); err != nil {
		return repository.Grant{}, err
	}
	return grant, nil
}

func (s *RepositoryAccessStore) ListGrants(ctx context.Context, projectID string, includeRevoked bool) ([]repository.Grant, error) {
	query := `SELECT id, project_id, root_path, canonical_root, created_at, revoked_at FROM repository_grants WHERE project_id = ?`
	if !includeRevoked {
		query += ` AND revoked_at IS NULL`
	}
	query += ` ORDER BY created_at DESC, id`
	rows, err := s.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list repository grants: %w", err)
	}
	defer rows.Close()
	grants := make([]repository.Grant, 0)
	for rows.Next() {
		grant, scanErr := scanRepositoryGrant(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s *RepositoryAccessStore) GetGrant(ctx context.Context, projectID, grantID string) (repository.Grant, error) {
	grant, err := scanRepositoryGrant(s.db.QueryRowContext(ctx, `SELECT id, project_id, root_path, canonical_root, created_at, revoked_at FROM repository_grants WHERE project_id = ? AND id = ?`, projectID, grantID))
	if errors.Is(err, sql.ErrNoRows) {
		return repository.Grant{}, fmt.Errorf("%w: repository grant %s", application.ErrNotFound, grantID)
	}
	return grant, err
}

func (s *RepositoryAccessStore) RevokeGrant(ctx context.Context, projectID, grantID string, revokedAt time.Time) (repository.Grant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return repository.Grant{}, err
	}
	defer tx.Rollback()
	grant, err := scanRepositoryGrant(tx.QueryRowContext(ctx, `SELECT id, project_id, root_path, canonical_root, created_at, revoked_at FROM repository_grants WHERE project_id = ? AND id = ?`, projectID, grantID))
	if errors.Is(err, sql.ErrNoRows) {
		return repository.Grant{}, fmt.Errorf("%w: repository grant %s", application.ErrNotFound, grantID)
	}
	if err != nil {
		return repository.Grant{}, err
	}
	if grant.RevokedAt != nil {
		return repository.Grant{}, fmt.Errorf("%w: repository grant is already revoked", application.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE repository_grants SET revoked_at = ? WHERE id = ?`, formatTime(revokedAt), grantID); err != nil {
		return repository.Grant{}, err
	}
	if err := insertEvent(ctx, tx, projectID, "repository.revoked", map[string]any{"grant_id": grantID}, revokedAt); err != nil {
		return repository.Grant{}, err
	}
	if err := tx.Commit(); err != nil {
		return repository.Grant{}, err
	}
	grant.RevokedAt = &revokedAt
	return grant, nil
}

func (s *RepositoryAccessStore) RecordToolCall(ctx context.Context, call repository.ToolCall) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO repository_tool_calls
		(id, project_id, grant_id, tool_name, arguments_checksum, result_checksum, status, error_code, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, call.ID, call.ProjectID, call.GrantID, call.Tool, call.ArgumentsChecksum, call.ResultChecksum, call.Status, call.ErrorCode, formatTime(call.StartedAt), nullableTime(call.CompletedAt))
	if err != nil {
		return fmt.Errorf("record repository tool call: %w", err)
	}
	if err := insertEvent(ctx, tx, call.ProjectID, "repository.tool_finished", map[string]any{"tool_call_id": call.ID, "grant_id": call.GrantID, "tool": call.Tool, "status": call.Status, "error_code": call.ErrorCode}, *call.CompletedAt); err != nil {
		return err
	}
	return tx.Commit()
}

type grantScanner interface {
	Scan(...any) error
}

func scanRepositoryGrant(scanner grantScanner) (repository.Grant, error) {
	var grant repository.Grant
	var createdAt string
	var revokedAt sql.NullString
	if err := scanner.Scan(&grant.ID, &grant.ProjectID, &grant.RootPath, &grant.CanonicalRoot, &createdAt, &revokedAt); err != nil {
		return repository.Grant{}, err
	}
	var err error
	grant.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return repository.Grant{}, err
	}
	if revokedAt.Valid {
		parsed, parseErr := parseTime(revokedAt.String)
		if parseErr != nil {
			return repository.Grant{}, parseErr
		}
		grant.RevokedAt = &parsed
	}
	return grant, nil
}
