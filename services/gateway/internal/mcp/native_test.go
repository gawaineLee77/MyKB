package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type nativeCallerCapture struct {
	actors  []nativeaccess.Actor
	headers []http.Header
}

type nativeAuditOnlyStore struct{ nativeaccess.SessionStore }

func (nativeAuditOnlyStore) Record(context.Context, nativeaccess.Actor, string, string, string, []string, string) error {
	return nil
}

func TestPlainMCPAskCannotInheritWorkspaceMemory(t *testing.T) {
	checks := 0
	s := &NativeService{Scopes: &nativeaccess.ScopeService{
		Store:           nativeAuditOnlyStore{},
		KnowledgeAccess: func(context.Context, string, http.Header) error { return nil },
		PlainChatAllowed: func(context.Context, http.Header) error {
			checks++
			return &nativeaccess.Error{Status: 403, Code: "kb_acl.workspace_memory_disabled_required"}
		},
	}}
	for i, body := range []string{`{"query":"synthetic"}`, `{"query":"synthetic","agent_id":"agent"}`} {
		_, err := s.CallNative(context.Background(), "ask_knowledge_agent", json.RawMessage(body), nativeaccess.Actor{ID: "user", Kind: "human", TenantID: 1}, http.Header{}, "request")
		if err != ErrDenied || checks != i+1 {
			t.Fatal("MCP ask did not fail before scope retrieval/model invocation", err, checks)
		}
	}
}

func TestReadOnlyMCPAgentCannotExecuteMutationTools(t *testing.T) {
	for _, config := range []map[string]any{
		{"allowed_tools": []any{"wiki_write_page"}},
		{"agent_mode": "quick-answer", "memory_enabled": true},
		{"allowed_tools": []any{"database_query"}},
		{"mcp_selection_mode": "all"},
		{"mcp_services": []any{"external"}},
		{"selected_skills": []any{"execute"}},
	} {
		if readOnlyAgent(config) {
			t.Fatal(config)
		}
	}
	if !readOnlyAgent(map[string]any{"allowed_tools": []any{"knowledge_search", "wiki_search"}, "mcp_selection_mode": "none"}) {
		t.Fatal("safe native tools rejected")
	}
}

func (c *nativeCallerCapture) CallNative(_ context.Context, _ string, _ json.RawMessage, a nativeaccess.Actor, h http.Header, _ string) (any, error) {
	c.actors = append(c.actors, a)
	c.headers = append(c.headers, h.Clone())
	return map[string]any{"ok": true}, nil
}

type nativeResolverStub struct{}

func (nativeResolverStub) CurrentPrincipal(_ context.Context, h http.Header) (weknora.Principal, error) {
	return weknora.Principal{User: &weknora.User{ID: "native-owner", IsActive: true, IsSystemAdmin: true}, Tenant: &weknora.Tenant{ID: 42}}, nil
}

type nativeLimiterCapture struct{ keys []string }

func (l *nativeLimiterCapture) Allow(key string, _ time.Time) bool {
	l.keys = append(l.keys, key)
	return true
}

func TestNativeMCPSixToolsAndMachineIdentity(t *testing.T) {
	calls := &nativeCallerCapture{}
	limiter := &nativeLimiterCapture{}
	h, err := NewNativeHandler(nativeResolverStub{}, calls, limiter, "r3-test")
	if err != nil {
		t.Fatal(err)
	}
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	r := modernRequest(t, "tools/list", "", `{`+meta+`}`)
	r.Header.Del("Authorization")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	r = modernRequest(t, "tools/list", "", `{`+meta+`}`)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var document struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if json.Unmarshal(w.Body.Bytes(), &document) != nil || len(document.Result.Tools) != 6 {
		t.Fatal(w.Body.String())
	}
	for _, tool := range document.Result.Tools {
		if !nativeToolName(tool.Name) {
			t.Fatal(tool.Name)
		}
	}
	for _, key := range []string{"synthetic-key-a", "synthetic-key-b"} {
		r = modernRequest(t, "tools/call", "list_knowledge_bases", `{"name":"list_knowledge_bases","arguments":{},`+meta+`}`)
		r.Header.Del("Authorization")
		r.Header.Set("X-API-Key", key)
		r.Header.Set("X-Tenant-ID", "42")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if len(calls.actors) != 2 || calls.actors[0].Kind != "api_key" || calls.actors[0].ID == "native-owner" || calls.actors[0].ID == calls.actors[1].ID {
		t.Fatal(calls.actors)
	}
	for i, a := range calls.actors {
		if strings.Contains(a.ID, "synthetic") || len(a.ID) != 64 || calls.headers[i].Get("X-API-Key") == "" {
			t.Fatal("unsafe or lost credential identity")
		}
	}
	if limiter.keys[len(limiter.keys)-1] == limiter.keys[len(limiter.keys)-2] {
		t.Fatal("machine rate buckets collide")
	}
	r = modernRequest(t, "tools/call", "list_publications", `{"name":"list_publications","arguments":{},`+meta+`}`)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"code":-32602`) || len(calls.actors) != 2 {
		t.Fatal(w.Body.String())
	}
}
