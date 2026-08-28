// Package subscription owns live follow relationships to MindCreek publications.
package subscription

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid         = errors.New("invalid subscription request")
	ErrNotFound        = errors.New("subscription not found")
	ErrUnavailable     = errors.New("subscription service unavailable")
	ErrPublication     = errors.New("publication unavailable")
	ErrOutsideAudience = errors.New("publication outside audience")
	ErrOwner           = errors.New("owners do not subscribe to their own publication")
)

type Status string

const (
	StatusActive       Status = "active"
	StatusInactive     Status = "inactive"
	StatusUnsubscribed Status = "unsubscribed"
)

type Subscription struct {
	ID                     string     `json:"id"`
	PublicationID          string     `json:"publication_id"`
	SubscriberID           string     `json:"subscriber_id"`
	SubscriberTenantID     uint64     `json:"subscriber_tenant_id"`
	Status                 Status     `json:"status"`
	NotificationEnabled    bool       `json:"notification_enabled"`
	LastSeenRevision       int64      `json:"last_seen_revision"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	EndedAt                *time.Time `json:"ended_at,omitempty"`
	LastAuditCorrelationID string     `json:"-"`
}

func (s Subscription) Active() bool { return s.Status == StatusActive && s.EndedAt == nil }

func (s Subscription) Validate() error {
	if strings.TrimSpace(s.ID) == "" || len(s.ID) > 36 || strings.TrimSpace(s.PublicationID) == "" || len(s.PublicationID) > 36 ||
		strings.TrimSpace(s.SubscriberID) == "" || len(s.SubscriberID) > 512 || s.SubscriberTenantID == 0 ||
		(s.Status != StatusActive && s.Status != StatusInactive && s.Status != StatusUnsubscribed) || s.LastSeenRevision < 0 ||
		s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() || s.UpdatedAt.Before(s.CreatedAt) ||
		strings.TrimSpace(s.LastAuditCorrelationID) == "" || len(s.LastAuditCorrelationID) > 128 {
		return ErrInvalid
	}
	if (s.Status == StatusActive && s.EndedAt != nil) || (s.Status != StatusActive && s.EndedAt == nil) {
		return ErrInvalid
	}
	if s.EndedAt != nil && s.EndedAt.Before(s.CreatedAt) {
		return ErrInvalid
	}
	return nil
}

type Actor struct {
	UserID   string
	TenantID uint64
}
