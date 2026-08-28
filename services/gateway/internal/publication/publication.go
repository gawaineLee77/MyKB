// Package publication owns MindCreek's internal publication and audience model.
package publication

import (
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalid          = errors.New("invalid publication")
	ErrNotFound         = errors.New("publication not found")
	ErrConflict         = errors.New("publication already exists")
	ErrRevisionConflict = errors.New("publication revision conflict")
	ErrNotOwner         = errors.New("publication owner required")
	ErrPersonalNotes    = errors.New("personal notes cannot be published")
	ErrUnavailable      = errors.New("publication service unavailable")
)

type AccessMode string

const (
	AccessSubscriber         AccessMode = "subscriber"
	AccessOrganizationPublic AccessMode = "organization_public"
)

func (m AccessMode) Valid() bool {
	return m == AccessSubscriber || m == AccessOrganizationPublic
}

type AudienceType string

const (
	AudienceOrganization AudienceType = "organization"
	AudienceWorkspaceSet AudienceType = "workspace_set"
)

type Audience struct {
	Type         AudienceType `json:"type"`
	WorkspaceIDs []uint64     `json:"workspace_ids,omitempty"`
}

func (a Audience) Valid() bool {
	if a.Type == AudienceOrganization {
		return len(a.WorkspaceIDs) == 0
	}
	if a.Type != AudienceWorkspaceSet || len(a.WorkspaceIDs) == 0 || len(a.WorkspaceIDs) > 100 {
		return false
	}
	seen := make(map[uint64]bool, len(a.WorkspaceIDs))
	for _, id := range a.WorkspaceIDs {
		if id == 0 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (a Audience) Allows(workspaceID uint64) bool {
	if workspaceID == 0 || !a.Valid() {
		return false
	}
	if a.Type == AudienceOrganization {
		return true
	}
	for _, id := range a.WorkspaceIDs {
		if id == workspaceID {
			return true
		}
	}
	return false
}

func (a Audience) normalized() Audience {
	result := Audience{Type: a.Type, WorkspaceIDs: append([]uint64(nil), a.WorkspaceIDs...)}
	sort.Slice(result.WorkspaceIDs, func(i, j int) bool { return result.WorkspaceIDs[i] < result.WorkspaceIDs[j] })
	return result
}

type Status string

const (
	StatusPublished   Status = "published"
	StatusUnpublished Status = "unpublished"
)

type Publication struct {
	ID                     string     `json:"id"`
	KnowledgeBaseID        string     `json:"knowledge_base_id"`
	PublisherID            string     `json:"publisher_id"`
	PublisherTenantID      uint64     `json:"publisher_tenant_id"`
	Title                  string     `json:"title"`
	Description            string     `json:"description"`
	Tags                   []string   `json:"tags"`
	UsageGuidance          string     `json:"usage_guidance"`
	Audience               Audience   `json:"audience"`
	AccessMode             AccessMode `json:"access_mode"`
	Status                 Status     `json:"status"`
	PublishedRevision      int64      `json:"published_revision"`
	CreatedAt              time.Time  `json:"created_at"`
	PublishedAt            time.Time  `json:"published_at"`
	UnpublishedAt          *time.Time `json:"unpublished_at,omitempty"`
	UpdatedAt              time.Time  `json:"updated_at"`
	RowVersion             int64      `json:"row_version"`
	LastAuditCorrelationID string     `json:"-"`
}

func (p Publication) Published() bool {
	return p.Status == StatusPublished && p.UnpublishedAt == nil
}

func (p Publication) VisibleTo(workspaceID uint64) bool {
	return p.Published() && p.Audience.Allows(workspaceID)
}

func (p Publication) Validate() error {
	if strings.TrimSpace(p.ID) == "" || len(p.ID) > 36 || strings.TrimSpace(p.KnowledgeBaseID) == "" || len(p.KnowledgeBaseID) > 36 ||
		strings.TrimSpace(p.PublisherID) == "" || len(p.PublisherID) > 512 || p.PublisherTenantID == 0 ||
		strings.TrimSpace(p.Title) == "" || len([]rune(p.Title)) > 160 || len([]rune(p.Description)) > 2000 ||
		len([]rune(p.UsageGuidance)) > 2000 || len(p.Tags) > 20 || !p.Audience.Valid() || !p.AccessMode.Valid() ||
		(p.Status != StatusPublished && p.Status != StatusUnpublished) || p.PublishedRevision < 1 || p.RowVersion < 1 ||
		p.CreatedAt.IsZero() || p.PublishedAt.IsZero() || p.UpdatedAt.IsZero() || p.PublishedAt.Before(p.CreatedAt) ||
		p.UpdatedAt.Before(p.CreatedAt) || strings.TrimSpace(p.LastAuditCorrelationID) == "" || len(p.LastAuditCorrelationID) > 128 {
		return ErrInvalid
	}
	if (p.Status == StatusPublished && p.UnpublishedAt != nil) || (p.Status == StatusUnpublished && p.UnpublishedAt == nil) {
		return ErrInvalid
	}
	if p.UnpublishedAt != nil && p.UnpublishedAt.Before(p.PublishedAt) {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(p.Tags))
	for _, tag := range p.Tags {
		if strings.TrimSpace(tag) != tag || tag == "" || len([]rune(tag)) > 40 || seen[tag] {
			return ErrInvalid
		}
		seen[tag] = true
	}
	return nil
}

type Actor struct {
	UserID   string
	TenantID uint64
}

type WriteRequest struct {
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	Tags               []string   `json:"tags"`
	UsageGuidance      string     `json:"usage_guidance"`
	Audience           Audience   `json:"audience"`
	AccessMode         AccessMode `json:"access_mode"`
	ExpectedRowVersion int64      `json:"expected_row_version,omitempty"`
	CorrelationID      string     `json:"-"`
}
