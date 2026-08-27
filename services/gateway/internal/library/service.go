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
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const MaxPageSize = 100

var (
	ErrInvalid     = errors.New("invalid library request")
	ErrUnavailable = errors.New("authorized library is unavailable")
)

type View string

const (
	ViewOwned  View = "owned"
	ViewShared View = "shared"
)

type Upstream interface {
	ListKnowledgeBases(context.Context, http.Header) ([]weknora.KnowledgeBase, error)
}

type Decisions interface {
	Decide(context.Context, string, authorization.Principal, http.Header) (authorization.Decision, error)
}

type Item struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Type        string              `json:"type"`
	CreatorID   string              `json:"creator_id"`
	Role        authorization.Role  `json:"role"`
	ProductMode profile.ProductMode `json:"product_mode,omitempty"`
}

type Page struct {
	Items    []Item `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type Service struct {
	upstream  Upstream
	decisions Decisions
}

func NewService(upstream Upstream, decisions Decisions) (*Service, error) {
	if upstream == nil || decisions == nil {
		return nil, fmt.Errorf("upstream list and authorization decisions are required")
	}
	return &Service{upstream: upstream, decisions: decisions}, nil
}

func (s *Service) List(ctx context.Context, view View, page, pageSize int, principal authorization.Principal, headers http.Header) (Page, error) {
	if (view != ViewOwned && view != ViewShared) || page < 1 || pageSize < 1 || pageSize > MaxPageSize ||
		strings.TrimSpace(principal.UserID) == "" || principal.TenantID == 0 {
		return Page{}, ErrInvalid
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
			(view == ViewShared && decision.Role != authorization.RoleViewer && decision.Role != authorization.RoleEditor) {
			continue
		}
		authorized = append(authorized, Item{
			ID: kb.ID, Name: kb.Name, Description: kb.Description, Type: kb.Type,
			CreatorID: kb.CreatorID, Role: decision.Role, ProductMode: decision.ProductMode,
		})
	}
	start := (page - 1) * pageSize
	if start > len(authorized) {
		start = len(authorized)
	}
	end := start + pageSize
	if end > len(authorized) {
		end = len(authorized)
	}
	return Page{Items: authorized[start:end], Total: len(authorized), Page: page, PageSize: pageSize}, nil
}
