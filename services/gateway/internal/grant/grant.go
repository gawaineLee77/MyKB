// Package grant stores and manages explicit MindCreek KB grants.
package grant

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid            = errors.New("invalid knowledge-base grant")
	ErrNotFound           = errors.New("knowledge-base grant not found")
	ErrConflict           = errors.New("knowledge-base grant already exists")
	ErrRevisionConflict   = errors.New("knowledge-base grant revision conflict")
	ErrNotOwner           = errors.New("only the knowledge-base owner may manage grants")
	ErrPersonalNotes      = errors.New("Personal Notes cannot be shared")
	ErrSubjectUnsupported = errors.New("grant subject type is not enabled")
	ErrAuditUnavailable   = errors.New("grant audit record is unavailable")
)

type SubjectType string

const (
	SubjectUser      SubjectType = "user"
	SubjectGroup     SubjectType = "group"
	SubjectWorkspace SubjectType = "workspace"
)

func (s SubjectType) Valid() bool {
	switch s {
	case SubjectUser, SubjectGroup, SubjectWorkspace:
		return true
	default:
		return false
	}
}

type Permission string

const (
	PermissionViewer Permission = "viewer"
	PermissionEditor Permission = "editor"
)

func (p Permission) Valid() bool {
	return p == PermissionViewer || p == PermissionEditor
}

type Grant struct {
	ID                     string      `json:"id"`
	KnowledgeBaseID        string      `json:"knowledge_base_id"`
	SubjectType            SubjectType `json:"subject_type"`
	SubjectID              string      `json:"subject_id"`
	Permission             Permission  `json:"permission"`
	GrantedBy              string      `json:"granted_by"`
	CreatedAt              time.Time   `json:"created_at"`
	UpdatedAt              time.Time   `json:"updated_at"`
	ExpiresAt              *time.Time  `json:"expires_at,omitempty"`
	RevokedAt              *time.Time  `json:"revoked_at,omitempty"`
	Revision               int64       `json:"revision"`
	LastAuditCorrelationID string      `json:"last_audit_correlation_id"`
}

func (g Grant) Active(at time.Time) bool {
	return g.RevokedAt == nil && (g.ExpiresAt == nil || g.ExpiresAt.After(at))
}

func (g Grant) validate() error {
	if strings.TrimSpace(g.ID) == "" || strings.TrimSpace(g.KnowledgeBaseID) == "" ||
		strings.TrimSpace(g.SubjectID) == "" || strings.TrimSpace(g.GrantedBy) == "" ||
		strings.TrimSpace(g.LastAuditCorrelationID) == "" || !g.SubjectType.Valid() || !g.Permission.Valid() {
		return ErrInvalid
	}
	if g.Revision < 1 || g.CreatedAt.IsZero() || g.UpdatedAt.IsZero() || g.UpdatedAt.Before(g.CreatedAt) {
		return ErrInvalid
	}
	if g.ExpiresAt != nil && !g.ExpiresAt.After(g.CreatedAt) {
		return ErrInvalid
	}
	if g.RevokedAt != nil && g.RevokedAt.Before(g.CreatedAt) {
		return ErrInvalid
	}
	return nil
}
