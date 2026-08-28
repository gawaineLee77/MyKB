// Package catalog exposes audience-filtered internal publications.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
)

const MaxPageSize = 100

var (
	ErrInvalid     = errors.New("invalid catalog request")
	ErrNotFound    = errors.New("catalog publication not found")
	ErrUnavailable = errors.New("catalog unavailable")
)

type Principal struct {
	UserID   string
	TenantID uint64
}

type Filter struct {
	Query        string
	Tag          string
	Owner        string
	AccessMode   publication.AccessMode
	UpdatedAfter *time.Time
	Page         int
	PageSize     int
}

type Item struct {
	Publication      publication.Publication `json:"publication"`
	CurrentRevision  int64                   `json:"current_revision"`
	Subscribed       bool                    `json:"subscribed"`
	LastSeenRevision int64                   `json:"last_seen_revision,omitempty"`
	Updated          bool                    `json:"updated"`
	CanRead          bool                    `json:"can_read"`
	CanSubscribe     bool                    `json:"can_subscribe"`
}

type Page struct {
	Items    []Item `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type PublicationStore interface {
	Get(context.Context, string) (publication.Publication, error)
	ListPublished(context.Context) ([]publication.Publication, error)
}

type SubscriptionReader interface {
	Effective(context.Context, string, string, uint64) (subscription.Subscription, error)
}

type RevisionReader interface {
	Current(context.Context, string) (int64, error)
}

type Service struct {
	publications  PublicationStore
	subscriptions SubscriptionReader
	revisions     RevisionReader
}

func NewService(publications PublicationStore, subscriptions SubscriptionReader, revisions RevisionReader) (*Service, error) {
	if publications == nil || subscriptions == nil || revisions == nil {
		return nil, fmt.Errorf("catalog publication, subscription, and revision readers are required")
	}
	return &Service{publications: publications, subscriptions: subscriptions, revisions: revisions}, nil
}

func (s *Service) List(ctx context.Context, principal Principal, filter Filter) (Page, error) {
	if !validPrincipal(principal) || !validFilter(filter) {
		return Page{}, ErrInvalid
	}
	publications, err := s.publications.ListPublished(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	matched := make([]Item, 0, len(publications))
	for _, candidate := range publications {
		if candidate.PublisherID != principal.UserID && !candidate.VisibleTo(principal.TenantID) {
			continue
		}
		if !matches(candidate, filter) {
			continue
		}
		item, err := s.decorate(ctx, principal, candidate)
		if err != nil {
			return Page{}, err
		}
		matched = append(matched, item)
	}
	start := len(matched)
	if filter.Page-1 <= len(matched)/filter.PageSize {
		start = (filter.Page - 1) * filter.PageSize
	}
	end := start + filter.PageSize
	if end > len(matched) {
		end = len(matched)
	}
	return Page{Items: matched[start:end], Total: len(matched), Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, principal Principal, publicationID string) (Item, error) {
	if !validPrincipal(principal) || strings.TrimSpace(publicationID) == "" {
		return Item{}, ErrInvalid
	}
	value, err := s.publications.Get(ctx, publicationID)
	if errors.Is(err, publication.ErrNotFound) || err == nil && (!value.Published() || value.PublisherID != principal.UserID && !value.VisibleTo(principal.TenantID)) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return s.decorate(ctx, principal, value)
}

func (s *Service) decorate(ctx context.Context, principal Principal, value publication.Publication) (Item, error) {
	current, err := s.revisions.Current(ctx, value.KnowledgeBaseID)
	if err != nil {
		return Item{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	item := Item{Publication: value, CurrentRevision: current, CanRead: value.PublisherID == principal.UserID || value.AccessMode == publication.AccessOrganizationPublic}
	effective, err := s.subscriptions.Effective(ctx, value.ID, principal.UserID, principal.TenantID)
	if err == nil {
		item.Subscribed = true
		item.LastSeenRevision = effective.LastSeenRevision
		item.Updated = current > effective.LastSeenRevision
		item.CanRead = true
	} else if !errors.Is(err, subscription.ErrNotFound) {
		return Item{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	item.CanSubscribe = value.PublisherID != principal.UserID && !item.Subscribed
	return item, nil
}

func validPrincipal(value Principal) bool {
	return strings.TrimSpace(value.UserID) != "" && value.TenantID > 0
}

func validFilter(value Filter) bool {
	return value.Page > 0 && value.PageSize > 0 && value.PageSize <= MaxPageSize &&
		len([]rune(value.Query)) <= 160 && len([]rune(value.Tag)) <= 40 && len(value.Owner) <= 512 &&
		(value.AccessMode == "" || value.AccessMode.Valid())
}

func matches(value publication.Publication, filter Filter) bool {
	if filter.Owner != "" && value.PublisherID != filter.Owner {
		return false
	}
	if filter.AccessMode != "" && value.AccessMode != filter.AccessMode {
		return false
	}
	if filter.UpdatedAfter != nil && !value.UpdatedAt.After(*filter.UpdatedAfter) {
		return false
	}
	if filter.Tag != "" {
		wanted := strings.ToLower(strings.TrimSpace(filter.Tag))
		found := false
		for _, tag := range value.Tags {
			if tag == wanted {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if filter.Query != "" {
		query := strings.ToLower(strings.TrimSpace(filter.Query))
		haystack := strings.ToLower(value.Title + "\n" + value.Description + "\n" + value.UsageGuidance + "\n" + strings.Join(value.Tags, " "))
		if !strings.Contains(haystack, query) {
			return false
		}
	}
	return true
}
