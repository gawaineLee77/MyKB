package nativeaccess

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type memoryStore struct {
	mu       sync.Mutex
	bindings map[string]Binding
	events   int
}

func (m *memoryStore) Bind(_ context.Context, id string, a Actor, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bindings == nil {
		m.bindings = map[string]Binding{}
	}
	b, ok := m.bindings[id]
	if ok && b.Actor != a {
		return &Error{403, "session.principal_mismatch"}
	}
	b.SessionID = id
	b.Actor = a
	b.KBIDs = unique(append(b.KBIDs, ids...))
	m.bindings[id] = b
	return nil
}
func (m *memoryStore) Binding(_ context.Context, id string) (Binding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bindings[id]
	if !ok {
		return b, &Error{403, "session.unbound"}
	}
	return b, nil
}
func (m *memoryStore) Bindings(_ context.Context, a Actor) ([]Binding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Binding{}
	for _, b := range m.bindings {
		if b.Actor == a {
			out = append(out, b)
		}
	}
	return out, nil
}
func (m *memoryStore) Record(context.Context, Actor, string, string, string, []string, string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events++
	return nil
}

type nativeFixture struct {
	graphEngine string
	allowed     map[string]bool
	requests    []string
	agent       map[string]any
	caps        map[string]map[string]any
}

func (f *nativeFixture) NativeRequest(_ context.Context, method, path string, _ url.Values, h http.Header, _ any) (json.RawMessage, error) {
	f.requests = append(f.requests, method+" "+path)
	var data any
	switch {
	case path == "/api/v1/system/info":
		data = map[string]any{"graph_database_engine": f.graphEngine}
	case path == "/api/v1/knowledge-bases":
		data = []any{}
		for id, ok := range f.allowed {
			if ok {
				data = append(data.([]any), map[string]any{"id": id})
			}
		}
	case path == "/api/v1/shared-knowledge-bases":
		data = []any{}
	case strings.HasPrefix(path, "/api/v1/knowledge-bases/"):
		id := strings.TrimPrefix(path, "/api/v1/knowledge-bases/")
		if !f.allowed[id] {
			return nil, &weknora.Error{StatusCode: 403, Code: "upstream.forbidden"}
		}
		data = map[string]any{"id": id, "capabilities": f.caps[id]}
	case strings.HasPrefix(path, "/api/v1/agents/"):
		data = f.agent
	case strings.HasPrefix(path, "/api/v1/models/"):
		id := strings.TrimPrefix(path, "/api/v1/models/")
		kind := map[string]string{"builtin-mindcreek-chat": "KnowledgeQA", "builtin-mindcreek-embedding": "Embedding", "builtin-mindcreek-rerank": "Rerank", "existing-model": "KnowledgeQA"}[id]
		if kind == "" {
			return nil, &weknora.Error{StatusCode: 404, Code: "upstream.not_found"}
		}
		data = map[string]any{"id": id, "status": "active", "type": kind}
	default:
		return nil, &weknora.Error{StatusCode: 404, Code: "upstream.not_found"}
	}
	return json.Marshal(map[string]any{"success": true, "data": data})
}
func (f *nativeFixture) KnowledgeBaseForKnowledge(_ context.Context, id string, _ http.Header) (string, error) {
	if id == "private-file" {
		return "private", nil
	}
	return "a", nil
}
func (f *nativeFixture) KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error) {
	return "a", nil
}
func (f *nativeFixture) ValidateSession(context.Context, string, http.Header) error { return nil }
func requestJSON(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer synthetic")
	return r
}
func testScope() (*ScopeService, *nativeFixture, *memoryStore, Actor) {
	f := &nativeFixture{allowed: map[string]bool{"a": true, "b": true}, agent: map[string]any{"tenant_id": float64(1), "config": map[string]any{"agent_mode": "quick-answer", "kb_selection_mode": "selected", "knowledge_bases": []any{"a"}}}}
	store := &memoryStore{}
	return &ScopeService{Upstream: f, Store: store}, f, store, Actor{Kind: "human", ID: "employee", TenantID: 1}
}

func TestExplicitScopeAndFileCannotExpandAgent(t *testing.T) {
	s, _, store, actor := testScope()
	for _, body := range []string{`{"knowledge_base_ids":["a","private"]}`, `{"knowledge_base_ids":["a"],"knowledge_ids":["private-file"]}`, `{"knowledge_base_ids":["b"],"agent_id":"agent"}`, `{"mentioned_items":[{"type":"file","id":"private-file","kb_id":"a"}]}`} {
		r := requestJSON("POST", "/api/v1/knowledge-search", body)
		if err := s.Check(r.Context(), r, actor); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	if store.events != 4 {
		t.Fatal("denials were not audited")
	}
}

func TestDefaultSelectionAndSessionRevocation(t *testing.T) {
	s, f, store, actor := testScope()
	if err := store.Bind(context.Background(), "session", actor, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	r := requestJSON("POST", "/api/v1/knowledge-chat/session", `{"query":"synthetic","agent_id":"agent"}`)
	if err := s.Check(r.Context(), r, actor); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	json.NewDecoder(r.Body).Decode(&payload)
	if ids := stringsValue(payload["knowledge_base_ids"]); len(ids) != 1 || ids[0] != "a" {
		t.Fatal(payload)
	}
	f.allowed["a"] = false
	for _, path := range []string{"/api/v1/sessions/session", "/api/v1/messages/session/load", "/api/v1/sessions/continue-stream/session", "/api/v1/sessions/session/messages/m/files"} {
		r := requestJSON("GET", path, "")
		if err := s.Check(r.Context(), r, actor); err == nil {
			t.Error(path)
		}
	}
	f.allowed["b"] = false
	if _, err := s.Resolve(context.Background(), nil, false, "", actor, http.Header{}); err == nil {
		t.Fatal("empty default scope must fail")
	}
}

func TestSessionNeverAdoptsOldOrOtherCredential(t *testing.T) {
	s, _, store, actor := testScope()
	ctx := context.Background()
	if _, err := s.CheckSession(ctx, "old", actor, http.Header{}); err == nil {
		t.Fatal("adopted legacy session")
	}
	store.Bind(ctx, "new", actor, nil)
	for _, other := range []Actor{{Kind: "api_key", ID: actor.ID, TenantID: 1}, {Kind: "human", ID: "other", TenantID: 1}, {Kind: "human", ID: actor.ID, TenantID: 2}} {
		if _, err := s.CheckSession(ctx, "new", other, http.Header{}); err == nil {
			t.Fatal(other)
		}
	}
}

func TestResponseBindsCreationAndStopsForeignSSEEvent(t *testing.T) {
	s, _, store, actor := testScope()
	r := requestJSON("POST", "/api/v1/sessions", `{"title":"synthetic"}`)
	if err := s.Check(r.Context(), r, actor); err != nil {
		t.Fatal(err)
	}
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":"new"}}`)), Request: r}
	if err := s.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	if b, err := store.Binding(context.Background(), "new"); err != nil || b.Actor != actor {
		t.Fatal(b, err)
	}
	stream := &checkedStream{source: io.NopCloser(strings.NewReader("")), check: func(value any) error {
		return s.checkReferences(context.Background(), value, &requestState{Actor: actor, KBIDs: []string{"a"}, StrictScope: true}, http.Header{})
	}}
	stream.reader = bufio.NewReader(strings.NewReader("data: {\"response_type\":\"answer\",\"content\":\"safe\"}\n\ndata: {\"knowledge_base_id\":\"b\"}\n\n"))
	output, err := io.ReadAll(stream)
	if err == nil || strings.Contains(string(output), `"b"`) {
		t.Fatalf("foreign event exposed: %s %v", output, err)
	}
}
