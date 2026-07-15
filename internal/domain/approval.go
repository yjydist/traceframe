package domain

import (
	"fmt"
	"strings"
	"time"
)

type ApprovalStatus string

const (
	ApprovalPending     ApprovalStatus = "pending"
	ApprovalApproved    ApprovalStatus = "approved"
	ApprovalRejected    ApprovalStatus = "rejected"
	ApprovalInvalidated ApprovalStatus = "invalidated"
)

type Approval struct {
	ID              string         `json:"id"`
	ProjectID       string         `json:"project_id"`
	SubjectID       string         `json:"subject_id"`
	SubjectRevision int64          `json:"subject_revision"`
	ProjectRevision int64          `json:"project_revision"`
	Status          ApprovalStatus `json:"status"`
	RequestedBy     string         `json:"requested_by"`
	ResolvedBy      string         `json:"resolved_by,omitempty"`
	Rationale       string         `json:"rationale,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	ResolvedAt      *time.Time     `json:"resolved_at,omitempty"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func ValidateApproval(approval Approval) error {
	if strings.TrimSpace(approval.ID) == "" || strings.TrimSpace(approval.ProjectID) == "" || strings.TrimSpace(approval.SubjectID) == "" || strings.TrimSpace(approval.RequestedBy) == "" {
		return fmt.Errorf("%w: approval id, project_id, subject_id, and requested_by are required", ErrInvalid)
	}
	if approval.SubjectRevision < 1 || approval.ProjectRevision < 1 {
		return fmt.Errorf("%w: approval revisions must be positive", ErrInvalid)
	}
	switch approval.Status {
	case ApprovalPending, ApprovalApproved, ApprovalRejected, ApprovalInvalidated:
	default:
		return fmt.Errorf("%w: unsupported approval status %q", ErrInvalid, approval.Status)
	}
	return nil
}
