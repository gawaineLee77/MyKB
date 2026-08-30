package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentscope"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/catalog"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type scopeStub struct {
	result   agentscope.Result
	err      error
	requests []agentscope.Request
}

func (s *scopeStub) Resolve(_ context.Context, request agentscope.Request, _ authorization.Principal, _ http.Header) (agentscope.Result, error) {
	s.requests = append(s.requests, request)
	return s.result, s.err
}

type libraryStub struct{}

func (libraryStub) List(context.Context, library.View, int, int, authorization.Principal, http.Header) (library.Page, error) {
	return library.Page{}, nil
}

type catalogStub struct{}

func (catalogStub) List(context.Context, catalog.Principal, catalog.Filter) (catalog.Page, error) {
	return catalog.Page{}, nil
}

type subscriptionsStub struct{}

func (subscriptionsStub) List(context.Context, subscription.Actor) ([]subscription.Item, error) {
	return nil, nil
}

type knowledgeStub struct {
	searchRequest weknora.SearchRequest
	results       []weknora.SearchResult
	excerpt       weknora.ChunkExcerpt
	answer        weknora.AgentAnswer
}

func (s *knowledgeStub) SearchKnowledge(_ context.Context, input weknora.SearchRequest, _ http.Header) ([]weknora.SearchResult, error) {
	s.searchRequest = input
	return s.results, nil
}
func (s *knowledgeStub) KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error) {
	return s.excerpt.KnowledgeBaseID, nil
}
func (s *knowledgeStub) GetChunkExcerpt(context.Context, string, http.Header) (weknora.ChunkExcerpt, error) {
	return s.excerpt, nil
}
func (s *knowledgeStub) CreateChatSession(context.Context, string, http.Header) (weknora.ChatSession, error) {
	return weknora.ChatSession{ID: "session-1"}, nil
}
func (s *knowledgeStub) AskKnowledge(context.Context, string, string, string, []string, http.Header) (weknora.AgentAnswer, error) {
	return s.answer, nil
}

type auditStub struct{ events []agentaudit.Event }

func (s *auditStub) Record(_ context.Context, event agentaudit.Event) error {
	s.events = append(s.events, event)
	return nil
}

func TestSearchUsesResolvedScopeAndAuditsWithoutQuery(t *testing.T) {
	scopes := &scopeStub{result: agentscope.Result{Selection: agentscope.SelectionExplicit, KnowledgeBaseIDs: []string{"kb-a"}}}
	knowledge := &knowledgeStub{results: []weknora.SearchResult{{ID: "chunk-1", KnowledgeBaseID: "kb-a", KnowledgeID: "doc-1", Content: "grounded"}}}
	auditor := &auditStub{}
	service := mustService(t, scopes, knowledge, auditor)
	value, err := service.Call(context.Background(), "search_knowledge", json.RawMessage(`{"query":"private question","knowledge_base_ids":["kb-a"]}`), authorization.Principal{UserID: "alice", TenantID: 42}, nil, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if value == nil || len(knowledge.searchRequest.KnowledgeBaseIDs) != 1 || knowledge.searchRequest.KnowledgeBaseIDs[0] != "kb-a" {
		t.Fatalf("search request = %+v", knowledge.searchRequest)
	}
	if len(auditor.events) != 1 || auditor.events[0].Operation != "search_knowledge" || len(auditor.events[0].KnowledgeBaseIDs) != 1 {
		t.Fatalf("audit events = %+v", auditor.events)
	}
}

func TestDeniedToolScopeIsAuditedAndNonDisclosing(t *testing.T) {
	scopes := &scopeStub{err: agentscope.ErrDenied}
	auditor := &auditStub{}
	service := mustService(t, scopes, &knowledgeStub{}, auditor)
	_, err := service.Call(context.Background(), "search_knowledge", json.RawMessage(`{"query":"x","knowledge_base_ids":["private"]}`), authorization.Principal{UserID: "alice", TenantID: 42}, nil, "request-2")
	if !errors.Is(err, ErrDenied) || len(auditor.events) != 1 || auditor.events[0].Outcome != agentaudit.OutcomeDenied {
		t.Fatalf("error = %v, audit = %+v", err, auditor.events)
	}
}

func TestSourceExcerptRequiresFreshExplicitScope(t *testing.T) {
	scopes := &scopeStub{result: agentscope.Result{Selection: agentscope.SelectionExplicit, KnowledgeBaseIDs: []string{"kb-a"}}}
	knowledge := &knowledgeStub{excerpt: weknora.ChunkExcerpt{ID: "chunk-1", KnowledgeBaseID: "kb-a", KnowledgeID: "doc-1", Content: "0123456789"}}
	service := mustService(t, scopes, knowledge, &auditStub{})
	value, err := service.Call(context.Background(), "get_source_excerpt", json.RawMessage(`{"chunk_id":"chunk-1","max_chars":4}`), authorization.Principal{UserID: "alice", TenantID: 42}, nil, "request-3")
	if err != nil {
		t.Fatal(err)
	}
	excerpt := value.(weknora.ChunkExcerpt)
	if excerpt.Content != "0123" || len(scopes.requests) != 1 || scopes.requests[0].Selection != agentscope.SelectionExplicit {
		t.Fatalf("excerpt = %+v, requests = %+v", excerpt, scopes.requests)
	}
}

func TestUnknownAndInvalidToolsAreAudited(t *testing.T) {
	auditor := &auditStub{}
	service := mustService(t, &scopeStub{}, &knowledgeStub{}, auditor)
	if _, err := service.Call(context.Background(), "delete_everything", json.RawMessage(`{}`), authorization.Principal{UserID: "alice", TenantID: 42}, nil, "request-4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown tool error = %v", err)
	}
	if _, err := service.Call(context.Background(), "search_knowledge", json.RawMessage(`{"query":""}`), authorization.Principal{UserID: "alice", TenantID: 42}, nil, "request-5"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid tool error = %v", err)
	}
	if len(auditor.events) != 2 || auditor.events[0].Outcome != agentaudit.OutcomeDenied || auditor.events[1].Outcome != agentaudit.OutcomeDenied {
		t.Fatalf("audit events = %+v", auditor.events)
	}
}

func mustService(t *testing.T, scopes ScopeResolver, knowledge Knowledge, auditor agentaudit.Recorder) *Service {
	t.Helper()
	service, err := NewService(scopes, libraryStub{}, catalogStub{}, subscriptionsStub{}, knowledge, auditor)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC) }
	return service
}
