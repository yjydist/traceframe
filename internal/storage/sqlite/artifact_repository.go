package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/yjydist/traceframe/internal/application"
	"github.com/yjydist/traceframe/internal/artifacts"
	"github.com/yjydist/traceframe/internal/review"
)

type ArtifactRepository struct{ db *sql.DB }

func NewArtifactRepository(db *sql.DB) *ArtifactRepository { return &ArtifactRepository{db: db} }

func (r *ArtifactRepository) EnsureArtifact(ctx context.Context, artifact artifacts.Artifact) (artifacts.Artifact, error) {
	_, err := r.db.ExecContext(ctx, `INSERT INTO artifacts (id, project_id, view_type, title, renderer_type, mandatory, concern, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(project_id, view_type, renderer_type) DO UPDATE SET title = excluded.title, mandatory = excluded.mandatory, concern = excluded.concern`, artifact.ID, artifact.ProjectID, artifact.ViewType, artifact.Title, artifact.RendererType, artifact.Mandatory, artifact.Concern, formatTime(artifact.CreatedAt))
	if err != nil {
		return artifacts.Artifact{}, fmt.Errorf("ensure artifact: %w", err)
	}
	return scanArtifact(r.db.QueryRowContext(ctx, artifactSelect+` WHERE project_id = ? AND view_type = ? AND renderer_type = ?`, artifact.ProjectID, artifact.ViewType, artifact.RendererType))
}

func (r *ArtifactRepository) SaveVersion(ctx context.Context, version artifacts.Version) (artifacts.Version, bool, error) {
	included, _ := json.Marshal(version.IncludedEntityIDs)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return artifacts.Version{}, false, fmt.Errorf("begin artifact version: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO artifact_versions (id, artifact_id, project_id, renderer_version, source_revision, source_baseline_id, included_entity_ids_json, content_type, content, checksum, generation_run_id, stale, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, version.ID, version.ArtifactID, version.ProjectID, version.RendererVersion, version.SourceRevision, nullableString(version.SourceBaselineID), string(included), version.ContentType, version.Content, version.Checksum, version.GenerationRunID, version.Stale, formatTime(version.CreatedAt))
	if err != nil {
		return artifacts.Version{}, false, fmt.Errorf("insert artifact version: %w", err)
	}
	created, _ := result.RowsAffected()
	if created == 0 {
		existing, err := scanArtifactVersion(tx.QueryRowContext(ctx, versionSelect+` WHERE artifact_versions.artifact_id = ? AND artifact_versions.source_revision = ? AND artifact_versions.renderer_version = ? AND artifact_versions.checksum = ?`, version.ArtifactID, version.SourceRevision, version.RendererVersion, version.Checksum))
		return existing, false, err
	}
	if err := insertEvent(ctx, tx, version.ProjectID, "artifact.rendered", map[string]any{"artifact_id": version.ArtifactID, "version_id": version.ID, "renderer": version.RendererType, "source_revision": version.SourceRevision, "checksum": version.Checksum}, version.CreatedAt); err != nil {
		return artifacts.Version{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return artifacts.Version{}, false, fmt.Errorf("commit artifact version: %w", err)
	}
	return version, true, nil
}

func (r *ArtifactRepository) ListArtifacts(ctx context.Context, projectID string) ([]artifacts.Artifact, error) {
	rows, err := r.db.QueryContext(ctx, artifactSelect+` WHERE project_id = ? ORDER BY view_type, renderer_type`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	result := make([]artifacts.Artifact, 0)
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, artifact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close artifacts: %w", err)
	}
	for index := range result {
		latest, err := scanArtifactVersion(r.db.QueryRowContext(ctx, versionSelect+` WHERE artifact_versions.artifact_id = ? ORDER BY artifact_versions.source_revision DESC, artifact_versions.created_at DESC LIMIT 1`, result[index].ID))
		if err != nil && !errors.Is(err, application.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			result[index].Latest = &latest
		}
	}
	return result, nil
}

func (r *ArtifactRepository) GetArtifact(ctx context.Context, projectID, artifactID string) (artifacts.Artifact, error) {
	artifact, err := scanArtifact(r.db.QueryRowContext(ctx, artifactSelect+` WHERE project_id = ? AND id = ?`, projectID, artifactID))
	if err != nil {
		return artifacts.Artifact{}, err
	}
	latest, err := scanArtifactVersion(r.db.QueryRowContext(ctx, versionSelect+` WHERE artifact_versions.artifact_id = ? ORDER BY artifact_versions.source_revision DESC, artifact_versions.created_at DESC LIMIT 1`, artifact.ID))
	if err == nil {
		artifact.Latest = &latest
	} else if !errors.Is(err, application.ErrNotFound) {
		return artifacts.Artifact{}, err
	}
	return artifact, nil
}

func (r *ArtifactRepository) GetBaseline(ctx context.Context, projectID, baselineID string) (review.Baseline, error) {
	return scanBaseline(r.db.QueryRowContext(ctx, baselineSelect+` WHERE project_id = ? AND id = ?`, projectID, baselineID))
}

func (r *ArtifactRepository) LatestBaseline(ctx context.Context, projectID string) (review.Baseline, bool, error) {
	baseline, err := scanBaseline(r.db.QueryRowContext(ctx, baselineSelect+` WHERE project_id = ? ORDER BY project_revision DESC LIMIT 1`, projectID))
	if errors.Is(err, application.ErrNotFound) {
		return review.Baseline{}, false, nil
	}
	return baseline, err == nil, err
}

func (r *ArtifactRepository) CurrentViewTypes(ctx context.Context, projectID string, sourceRevision int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT artifacts.view_type FROM artifact_versions JOIN artifacts ON artifacts.id = artifact_versions.artifact_id WHERE artifact_versions.project_id = ? AND artifact_versions.source_revision = ? AND artifact_versions.stale = 0 ORDER BY artifacts.view_type`, projectID, sourceRevision)
	if err != nil {
		return nil, fmt.Errorf("list current artifact views: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var viewType string
		if err := rows.Scan(&viewType); err != nil {
			return nil, fmt.Errorf("scan current artifact view: %w", err)
		}
		result = append(result, viewType)
	}
	return result, rows.Err()
}

const artifactSelect = `SELECT id, project_id, view_type, title, renderer_type, mandatory, concern, created_at FROM artifacts`

func scanArtifact(scanner approvalScanner) (artifacts.Artifact, error) {
	var artifact artifacts.Artifact
	var createdAt string
	if err := scanner.Scan(&artifact.ID, &artifact.ProjectID, &artifact.ViewType, &artifact.Title, &artifact.RendererType, &artifact.Mandatory, &artifact.Concern, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return artifacts.Artifact{}, fmt.Errorf("%w: artifact", application.ErrNotFound)
		}
		return artifacts.Artifact{}, fmt.Errorf("scan artifact: %w", err)
	}
	var err error
	artifact.CreatedAt, err = parseTime(createdAt)
	return artifact, err
}

const versionSelect = `SELECT artifact_versions.id, artifact_versions.artifact_id, artifact_versions.project_id, artifacts.renderer_type, artifact_versions.renderer_version, artifact_versions.source_revision, COALESCE(artifact_versions.source_baseline_id, ''), artifact_versions.included_entity_ids_json, artifact_versions.content_type, artifact_versions.content, artifact_versions.checksum, artifact_versions.generation_run_id, artifact_versions.stale, artifact_versions.created_at FROM artifact_versions JOIN artifacts ON artifacts.id = artifact_versions.artifact_id`

func scanArtifactVersion(scanner approvalScanner) (artifacts.Version, error) {
	var version artifacts.Version
	var included, createdAt string
	if err := scanner.Scan(&version.ID, &version.ArtifactID, &version.ProjectID, &version.RendererType, &version.RendererVersion, &version.SourceRevision, &version.SourceBaselineID, &included, &version.ContentType, &version.Content, &version.Checksum, &version.GenerationRunID, &version.Stale, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return artifacts.Version{}, fmt.Errorf("%w: artifact version", application.ErrNotFound)
		}
		return artifacts.Version{}, fmt.Errorf("scan artifact version: %w", err)
	}
	if err := json.Unmarshal([]byte(included), &version.IncludedEntityIDs); err != nil {
		return artifacts.Version{}, fmt.Errorf("decode included entity ids: %w", err)
	}
	var err error
	version.CreatedAt, err = parseTime(createdAt)
	return version, err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
