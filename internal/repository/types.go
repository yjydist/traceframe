package repository

import (
	"context"
	"time"
)

const (
	ToolListFiles          = "list_files"
	ToolSearchText         = "search_text"
	ToolReadFile           = "read_file"
	ToolInspectManifest    = "inspect_manifest"
	ToolInspectGitMetadata = "inspect_git_metadata"
	ToolListTests          = "list_tests"
)

var toolNames = []string{ToolListFiles, ToolSearchText, ToolReadFile, ToolInspectManifest, ToolInspectGitMetadata, ToolListTests}

type Grant struct {
	ID            string     `json:"id"`
	ProjectID     string     `json:"project_id"`
	RootPath      string     `json:"root_path"`
	CanonicalRoot string     `json:"canonical_root"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type ToolCall struct {
	ID                string     `json:"id"`
	ProjectID         string     `json:"project_id"`
	GrantID           string     `json:"grant_id"`
	Tool              string     `json:"tool"`
	ArgumentsChecksum string     `json:"arguments_checksum"`
	ResultChecksum    string     `json:"result_checksum,omitempty"`
	Status            string     `json:"status"`
	ErrorCode         string     `json:"error_code,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

type Store interface {
	CreateGrant(context.Context, Grant) (Grant, error)
	ListGrants(context.Context, string, bool) ([]Grant, error)
	GetGrant(context.Context, string, string) (Grant, error)
	RevokeGrant(context.Context, string, string, time.Time) (Grant, error)
	RecordToolCall(context.Context, ToolCall) error
}

type Options struct {
	MaxFileBytes   int64
	MaxResultBytes int
	MaxResults     int
	MaxWalkFiles   int
}

func DefaultOptions() Options {
	return Options{MaxFileBytes: 1 << 20, MaxResultBytes: 256 << 10, MaxResults: 100, MaxWalkFiles: 10_000}
}

type ExecuteRequest struct {
	GrantID          string `json:"grant_id"`
	Tool             string `json:"tool"`
	Path             string `json:"path,omitempty"`
	Query            string `json:"query,omitempty"`
	StartLine        int    `json:"start_line,omitempty"`
	EndLine          int    `json:"end_line,omitempty"`
	MaxResults       int    `json:"max_results,omitempty"`
	RecordEvidence   bool   `json:"record_evidence,omitempty"`
	ExpectedRevision int64  `json:"expected_revision,omitempty"`
	SubjectID        string `json:"subject_id,omitempty"`
}

type Entry struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Content   string `json:"content,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type ToolResult struct {
	Tool          string   `json:"tool"`
	GrantID       string   `json:"grant_id"`
	Entries       []Entry  `json:"entries"`
	EvidenceIDs   []string `json:"evidence_ids"`
	Truncated     bool     `json:"truncated"`
	Untrusted     bool     `json:"untrusted"`
	Policy        string   `json:"policy"`
	ModelRevision int64    `json:"model_revision,omitempty"`
}

type ImpactAnalysis struct {
	ProjectID               string   `json:"project_id"`
	ProjectRevision         int64    `json:"project_revision"`
	Mode                    string   `json:"mode"`
	SubjectID               string   `json:"subject_id,omitempty"`
	RepositoryEvidenceIDs   []string `json:"repository_evidence_ids"`
	DirectlyAffectedIDs     []string `json:"directly_affected_ids"`
	TransitivelyAffectedIDs []string `json:"transitively_affected_ids"`
	PotentiallyStaleIDs     []string `json:"potentially_stale_ids"`
}
