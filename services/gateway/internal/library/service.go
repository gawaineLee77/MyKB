// Package library exposes product-owned authorized knowledge-base views.
package library

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const MaxPageSize = 100

var (
	ErrInvalid     = errors.New("invalid library request")
	ErrUnavailable = errors.New("authorized library is unavailable")
)

type View string

const (
	ViewOwned      View = "owned"
	ViewShared     View = "shared"
	ViewSubscribed View = "subscribed"
	ViewAll        View = "all"
)

type Upstream interface {
	ListKnowledgeBases(context.Context, http.Header) ([]weknora.KnowledgeBase, error)
}

type Decisions interface {
	Decide(context.Context, string, authorization.Principal, http.Header) (authorization.Decision, error)
}

type Item struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Type             string               `json:"type"`
	CreatorID        string               `json:"creator_id"`
	Role             authorization.Role   `json:"role"`
	ProductMode      profile.ProductMode  `json:"product_mode,omitempty"`
	AccessSource     authorization.Source `json:"access_source"`
	PublicationID    string               `json:"publication_id,omitempty"`
	CurrentRevision  int64                `json:"current_revision,omitempty"`
	LastSeenRevision int64                `json:"last_seen_revision,omitempty"`
	Updated          bool                 `json:"updated,omitempty"`
}

type Page struct {
	Items    []Item `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type Service struct {
	upstream      Upstream
	decisions     Decisions
	subscriptions SubscriptionLister
}

type SubscriptionLister interface {
	List(context.Context, subscription.Actor) ([]subscription.Item, error)
}

type Option func(*Service)

func WithSubscriptions(subscriptions SubscriptionLister) Option {
	return func(service *Service) { service.subscriptions = subscriptions }
}

func NewService(upstream Upstream, decisions Decisions, options ...Option) (*Service, error) {
	if upstream == nil || decisions == nil {
		return nil, fmt.Errorf("upstream list and authorization decisions are required")
	}
	service := &Service{upstream: upstream, decisions: decisions}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) List(ctx context.Context, view View, page, pageSize int, principal authorization.Principal, headers http.Header) (Page, error) {
	if (view != ViewOwned && view != ViewShared && view != ViewSubscribed && view != ViewAll) || page < 1 || pageSize < 1 || pageSize > MaxPageSize ||
		strings.TrimSpace(principal.UserID) == "" || principal.TenantID == 0 {
		return Page{}, ErrInvalid
	}
	if (view == ViewSubscribed || view == ViewAll) && s.subscriptions == nil {
		return Page{}, ErrUnavailable
	}
	if view == ViewSubscribed {
		items, err := s.subscriptionItems(ctx, principal)
		if err != nil {
			return Page{}, err
		}
		return paginate(items, page, pageSize), nil
	}
	items, err := s.upstream.ListKnowledgeBases(ctx, headers)
	if err != nil {
		return Page{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	authorized := make([]Item, 0, len(items))
	for _, kb := range items {
		decision, err := s.decisions.Decide(ctx, kb.ID, principal, headers)
		if errors.Is(err, authorization.ErrNotFound) {
			continue
		}
		if err != nil {
			return Page{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		if (view == ViewOwned && decision.Role != authorization.RoleOwner) ||
			(view == ViewShared && decision.Source != authorization.SourceUserGrant) ||
			(view == ViewAll && decision.Source != authorization.SourceOwner && decision.Source != authorization.SourceUserGrant) {
			continue
		}
		authorized = append(authorized, Item{
			ID: kb.ID, Name: kb.Name, Description: kb.Description, Type: kb.Type,
			CreatorID: kb.CreatorID, Role: decision.Role, ProductMode: decision.ProductMode, AccessSource: decision.Source,
		})
	}
	if view == ViewAll {
		subscribed, err := s.subscriptionItems(ctx, principal)
		if err != nil {
			return Page{}, err
		}
		seen := make(map[string]bool, len(authorized))
		for _, item := range authorized {
			seen[item.ID] = true
		}
		for _, item := range subscribed {
			if !seen[item.ID] {
				authorized = append(authorized, item)
				seen[item.ID] = true
			}
		}
	}
	return paginate(authorized, page, pageSize), nil
}

func (s *Service) subscriptionItems(ctx context.Context, principal authorization.Principal) ([]Item, error) {
	items, err := s.subscriptions.List(ctx, subscription.Actor{UserID: principal.UserID, TenantID: principal.TenantID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	result := make([]Item, 0, len(items))
	for _, followed := range items {
		result = append(result, Item{
			ID: followed.Publication.KnowledgeBaseID, Name: followed.Publication.Title,
			Description: followed.Publication.Description, CreatorID: followed.Publication.PublisherID,
			Role: authorization.RoleViewer, ProductMode: profile.ModeRAG, AccessSource: authorization.SourceSubscription,
			PublicationID: followed.Publication.ID, CurrentRevision: followed.CurrentRevision,
			LastSeenRevision: followed.Subscription.LastSeenRevision, Updated: followed.Updated,
		})
	}
	return result, nil
}

func paginate(authorized []Item, page, pageSize int) Page {
	start := len(authorized)
	if page-1 <= len(authorized)/pageSize {
		start = (page - 1) * pageSize
	}
	end := start + pageSize
	if end > len(authorized) {
		end = len(authorized)
	}
	return Page{Items: authorized[start:end], Total: len(authorized), Page: page, PageSize: pageSize}
}
