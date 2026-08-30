// Package mcp exposes MindCreek domain services as a read-only MCP tool surface.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentscope"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/catalog"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

var (
	ErrInvalid     = errors.New("invalid MCP tool arguments")
	ErrDenied      = errors.New("MCP tool scope denied")
	ErrNotFound    = errors.New("MCP resource not found")
	ErrUnavailable = errors.New("MCP tool unavailable")
)

type ScopeResolver interface {
	Resolve(context.Context, agentscope.Request, authorization.Principal, http.Header) (agentscope.Result, error)
}

type Library interface {
	List(context.Context, library.View, int, int, authorization.Principal, http.Header) (library.Page, error)
}

type Catalog interface {
	List(context.Context, catalog.Principal, catalog.Filter) (catalog.Page, error)
}

type Subscriptions interface {
	List(context.Context, subscription.Actor) ([]subscription.Item, error)
}

type Knowledge interface {
	SearchKnowledge(context.Context, weknora.SearchRequest, http.Header) ([]weknora.SearchResult, error)
	KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error)
	GetChunkExcerpt(context.Context, string, http.Header) (weknora.ChunkExcerpt, error)
	CreateChatSession(context.Context, string, http.Header) (weknora.ChatSession, error)
	AskKnowledge(context.Context, string, string, string, []string, http.Header) (weknora.AgentAnswer, error)
}

type Service struct {
	scopes        ScopeResolver
	library       Library
	catalog       Catalog
	subscriptions Subscriptions
	knowledge     Knowledge
	auditor       agentaudit.Recorder
	now           func() time.Time
}

func NewService(scopes ScopeResolver, library Library, catalog Catalog, subscriptions Subscriptions, knowledge Knowledge, auditor agentaudit.Recorder) (*Service, error) {
	if scopes == nil || library == nil || catalog == nil || subscriptions == nil || knowledge == nil || auditor == nil {
		return nil, fmt.Errorf("MCP domain dependencies are required")
	}
	return &Service{scopes: scopes, library: library, catalog: catalog, subscriptions: subscriptions, knowledge: knowledge, auditor: auditor, now: time.Now}, nil
}

func (s *Service) Call(ctx context.Context, name string, arguments json.RawMessage, principal authorization.Principal, headers http.Header, correlationID string) (any, error) {
	started := s.now()
	var result any
	var scopeIDs []string
	var err error
	switch name {
	case "list_knowledge_bases":
		result, scopeIDs, err = s.listKnowledgeBases(ctx, arguments, principal, headers)
	case "search_knowledge":
		result, scopeIDs, err = s.searchKnowledge(ctx, arguments, principal, headers)
	case "get_source_excerpt":
		result, scopeIDs, err = s.getSourceExcerpt(ctx, arguments, principal, headers)
	case "ask_knowledge_agent":
		result, scopeIDs, err = s.askKnowledgeAgent(ctx, arguments, principal, headers)
	case "list_publications":
		result, scopeIDs, err = s.listPublications(ctx, arguments, principal)
	case "list_subscriptions":
		result, scopeIDs, err = s.listSubscriptions(ctx, arguments, principal)
	default:
		err = ErrNotFound
	}
	outcome, code := agentaudit.OutcomeSuccess, ""
	if err != nil {
		outcome, code = auditOutcome(err)
	}
	auditErr := s.auditor.Record(ctx, agentaudit.Event{
		TenantID: principal.TenantID, ActorUserID: principal.UserID, ClientKind: agentaudit.ClientMCP,
		Operation: name, KnowledgeBaseIDs: safeAuditScope(scopeIDs), Outcome: outcome, ErrorCode: code,
		CorrelationID: correlationID, Duration: s.now().Sub(started), CreatedAt: s.now().UTC(),
	})
	if auditErr != nil {
		return nil, fmt.Errorf("%w: audit: %v", ErrUnavailable, auditErr)
	}
	return result, err
}

func (s *Service) listKnowledgeBases(ctx context.Context, raw json.RawMessage, principal authorization.Principal, headers http.Header) (any, []string, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return nil, nil, err
	}
	page, err := s.library.List(ctx, library.ViewAll, 1, agentscope.MaxKnowledgeBases, principal, headers)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: library", ErrUnavailable)
	}
	if page.Total > agentscope.MaxKnowledgeBases {
		return nil, nil, ErrInvalid
	}
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return page, ids, nil
}

type scopedArguments struct {
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
}

func (s *Service) resolveScope(ctx context.Context, input scopedArguments, principal authorization.Principal, headers http.Header) (agentscope.Result, error) {
	request := agentscope.Request{Selection: agentscope.SelectionDefault}
	if len(input.KnowledgeBaseIDs) > 0 {
		request.Selection = agentscope.SelectionExplicit
		request.KnowledgeBaseIDs = input.KnowledgeBaseIDs
	}
	result, err := s.scopes.Resolve(ctx, request, principal, headers)
	if errors.Is(err, agentscope.ErrDenied) {
		return agentscope.Result{}, ErrDenied
	}
	if errors.Is(err, agentscope.ErrInvalid) || errors.Is(err, agentscope.ErrTooLarge) {
		return agentscope.Result{}, ErrInvalid
	}
	if err != nil {
		return agentscope.Result{}, fmt.Errorf("%w: scope", ErrUnavailable)
	}
	if len(result.KnowledgeBaseIDs) == 0 {
		return agentscope.Result{}, ErrInvalid
	}
	return result, nil
}

func (s *Service) searchKnowledge(ctx context.Context, raw json.RawMessage, principal authorization.Principal, headers http.Header) (any, []string, error) {
	var input struct {
		scopedArguments
		Query        string   `json:"query"`
		KnowledgeIDs []string `json:"knowledge_ids,omitempty"`
	}
	if err := decodeArguments(raw, &input); err != nil || strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > 4000 || len(input.KnowledgeIDs) > 64 {
		return nil, nil, ErrInvalid
	}
	scope, err := s.resolveScope(ctx, input.scopedArguments, principal, headers)
	if err != nil {
		return nil, input.KnowledgeBaseIDs, err
	}
	results, err := s.knowledge.SearchKnowledge(ctx, weknora.SearchRequest{Query: input.Query, KnowledgeBaseIDs: scope.KnowledgeBaseIDs, KnowledgeIDs: input.KnowledgeIDs}, headers)
	if err != nil {
		return nil, scope.KnowledgeBaseIDs, fmt.Errorf("%w: search", ErrUnavailable)
	}
	if len(results) > 20 {
		results = results[:20]
	}
	for index := range results {
		results[index].Content = truncate(results[index].Content, 4000)
	}
	return map[string]any{"results": results, "effective_scope": scope.KnowledgeBaseIDs}, scope.KnowledgeBaseIDs, nil
}

func (s *Service) getSourceExcerpt(ctx context.Context, raw json.RawMessage, principal authorization.Principal, headers http.Header) (any, []string, error) {
	var input struct {
		ChunkID  string `json:"chunk_id"`
		MaxChars int    `json:"max_chars,omitempty"`
	}
	if err := decodeArguments(raw, &input); err != nil || strings.TrimSpace(input.ChunkID) == "" || len(input.ChunkID) > 128 || input.MaxChars < 0 || input.MaxChars > 12000 {
		return nil, nil, ErrInvalid
	}
	kbID, err := s.knowledge.KnowledgeBaseForChunk(ctx, input.ChunkID, headers)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	if _, err := s.scopes.Resolve(ctx, agentscope.Request{Selection: agentscope.SelectionExplicit, KnowledgeBaseIDs: []string{kbID}}, principal, headers); err != nil {
		if errors.Is(err, agentscope.ErrDenied) || errors.Is(err, agentscope.ErrInvalid) {
			return nil, []string{kbID}, ErrNotFound
		}
		return nil, []string{kbID}, ErrUnavailable
	}
	excerpt, err := s.knowledge.GetChunkExcerpt(ctx, input.ChunkID, headers)
	if err != nil || excerpt.KnowledgeBaseID != kbID {
		return nil, []string{kbID}, ErrNotFound
	}
	limit := input.MaxChars
	if limit == 0 {
		limit = 4000
	}
	excerpt.Content = truncate(excerpt.Content, limit)
	return excerpt, []string{kbID}, nil
}

func (s *Service) askKnowledgeAgent(ctx context.Context, raw json.RawMessage, principal authorization.Principal, headers http.Header) (any, []string, error) {
	var input struct {
		scopedArguments
		Query     string `json:"query"`
		SessionID string `json:"session_id,omitempty"`
		AgentID   string `json:"agent_id,omitempty"`
	}
	if err := decodeArguments(raw, &input); err != nil || strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > 8000 || len(input.SessionID) > 128 || len(input.AgentID) > 128 {
		return nil, nil, ErrInvalid
	}
	scope, err := s.resolveScope(ctx, input.scopedArguments, principal, headers)
	if err != nil {
		return nil, input.KnowledgeBaseIDs, err
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		session, err := s.knowledge.CreateChatSession(ctx, "MindCreek MCP", headers)
		if err != nil {
			return nil, scope.KnowledgeBaseIDs, fmt.Errorf("%w: session", ErrUnavailable)
		}
		sessionID = session.ID
	}
	answer, err := s.knowledge.AskKnowledge(ctx, sessionID, input.Query, input.AgentID, scope.KnowledgeBaseIDs, headers)
	if err != nil {
		return nil, scope.KnowledgeBaseIDs, fmt.Errorf("%w: answer", ErrUnavailable)
	}
	return answer, scope.KnowledgeBaseIDs, nil
}

func (s *Service) listPublications(ctx context.Context, raw json.RawMessage, principal authorization.Principal) (any, []string, error) {
	var input struct {
		Query string `json:"query,omitempty"`
		Tag   string `json:"tag,omitempty"`
	}
	if err := decodeArguments(raw, &input); err != nil || utf8.RuneCountInString(input.Query) > 160 || utf8.RuneCountInString(input.Tag) > 40 {
		return nil, nil, ErrInvalid
	}
	page, err := s.catalog.List(ctx, catalog.Principal{UserID: principal.UserID, TenantID: principal.TenantID}, catalog.Filter{Query: input.Query, Tag: input.Tag, Page: 1, PageSize: 100})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: catalog", ErrUnavailable)
	}
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		if item.CanRead {
			ids = append(ids, item.Publication.KnowledgeBaseID)
		}
	}
	return page, unique(ids), nil
}

func (s *Service) listSubscriptions(ctx context.Context, raw json.RawMessage, principal authorization.Principal) (any, []string, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return nil, nil, err
	}
	items, err := s.subscriptions.List(ctx, subscription.Actor{UserID: principal.UserID, TenantID: principal.TenantID})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: subscriptions", ErrUnavailable)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Publication.KnowledgeBaseID)
	}
	return map[string]any{"items": items, "total": len(items)}, unique(ids), nil
}

func decodeArguments(raw json.RawMessage, destination any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte(`{}`)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

func truncate(value string, maxRunes int) string {
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}

func auditOutcome(err error) (agentaudit.Outcome, string) {
	switch {
	case errors.Is(err, ErrDenied), errors.Is(err, ErrNotFound):
		return agentaudit.OutcomeDenied, "resource.not_found"
	case errors.Is(err, ErrInvalid):
		return agentaudit.OutcomeDenied, "request.invalid"
	default:
		return agentaudit.OutcomeFailure, "operation.unavailable"
	}
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func safeAuditScope(values []string) []string {
	values = unique(values)
	if len(values) > agentscope.MaxKnowledgeBases {
		values = values[:agentscope.MaxKnowledgeBases]
	}
	return values
}
