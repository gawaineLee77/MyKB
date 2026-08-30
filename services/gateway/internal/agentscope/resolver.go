// Package agentscope computes the exact knowledge-base scope available to an agent request.
package agentscope

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
)

const MaxKnowledgeBases = 64

var (
	ErrInvalid     = errors.New("invalid agent scope")
	ErrDenied      = errors.New("agent scope denied")
	ErrTooLarge    = errors.New("agent scope exceeds the configured limit")
	ErrUnavailable = errors.New("agent scope is unavailable")
)

type Selection string

const (
	SelectionDefault  Selection = "default"
	SelectionExplicit Selection = "explicit"
)

type Request struct {
	Selection        Selection `json:"selection"`
	KnowledgeBaseIDs []string  `json:"knowledge_base_ids,omitempty"`
}

type Entry struct {
	KnowledgeBaseID string               `json:"knowledge_base_id"`
	Role            authorization.Role   `json:"role"`
	AccessSource    authorization.Source `json:"access_source"`
	ProductMode     string               `json:"product_mode,omitempty"`
}

type Result struct {
	Selection        Selection `json:"selection"`
	KnowledgeBaseIDs []string  `json:"knowledge_base_ids"`
	Entries          []Entry   `json:"entries"`
}

type Library interface {
	List(context.Context, library.View, int, int, authorization.Principal, http.Header) (library.Page, error)
}

type Decisions interface {
	Authorize(context.Context, string, authorization.Principal, authorization.Action, http.Header) (authorization.Decision, error)
}

type Resolver struct {
	library   Library
	decisions Decisions
}

func NewResolver(library Library, decisions Decisions) (*Resolver, error) {
	if library == nil || decisions == nil {
		return nil, fmt.Errorf("authorized library and decisions are required")
	}
	return &Resolver{library: library, decisions: decisions}, nil
}

func (r *Resolver) Resolve(ctx context.Context, request Request, principal authorization.Principal, headers http.Header) (Result, error) {
	if strings.TrimSpace(principal.UserID) == "" || principal.TenantID == 0 {
		return Result{}, ErrInvalid
	}
	switch request.Selection {
	case SelectionDefault:
		if len(request.KnowledgeBaseIDs) != 0 {
			return Result{}, ErrInvalid
		}
		return r.resolveDefault(ctx, principal, headers)
	case SelectionExplicit:
		return r.resolveExplicit(ctx, request.KnowledgeBaseIDs, principal, headers)
	default:
		return Result{}, ErrInvalid
	}
}

func (r *Resolver) resolveDefault(ctx context.Context, principal authorization.Principal, headers http.Header) (Result, error) {
	page, err := r.library.List(ctx, library.ViewAll, 1, MaxKnowledgeBases, principal, headers)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if page.Total > MaxKnowledgeBases {
		return Result{}, ErrTooLarge
	}
	entries := make([]Entry, 0, len(page.Items))
	for _, item := range page.Items {
		if item.AccessSource == authorization.SourceOrganizationPublic {
			// Organization-public access is readable, but default agent inclusion
			// requires an active subscription and is represented as such by ViewAll.
			continue
		}
		entries = append(entries, Entry{
			KnowledgeBaseID: item.ID,
			Role:            item.Role,
			AccessSource:    item.AccessSource,
			ProductMode:     string(item.ProductMode),
		})
	}
	return result(SelectionDefault, entries), nil
}

func (r *Resolver) resolveExplicit(ctx context.Context, ids []string, principal authorization.Principal, headers http.Header) (Result, error) {
	ids, err := normalizeIDs(ids)
	if err != nil {
		return Result{}, err
	}
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		decision, err := r.decisions.Authorize(ctx, id, principal, authorization.ActionRead, headers)
		if errors.Is(err, authorization.ErrDenied) || errors.Is(err, authorization.ErrNotFound) {
			return Result{}, ErrDenied
		}
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		entries = append(entries, Entry{
			KnowledgeBaseID: id,
			Role:            decision.Role,
			AccessSource:    decision.Source,
			ProductMode:     string(decision.ProductMode),
		})
	}
	return result(SelectionExplicit, entries), nil
}

func normalizeIDs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > MaxKnowledgeBases {
		if len(values) > MaxKnowledgeBases {
			return nil, ErrTooLarge
		}
		return nil, ErrInvalid
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 {
			return nil, ErrInvalid
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func result(selection Selection, entries []Entry) Result {
	sort.Slice(entries, func(i, j int) bool { return entries[i].KnowledgeBaseID < entries[j].KnowledgeBaseID })
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.KnowledgeBaseID)
	}
	return Result{Selection: selection, KnowledgeBaseIDs: ids, Entries: entries}
}
