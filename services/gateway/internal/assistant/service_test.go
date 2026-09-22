package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type fixture struct {
	channel            Channel
	role               string
	active, member, kb bool
	native             map[string]nativeaccess.Binding
	bindings           map[string]Binding
	calls              int
	budget             int
	lastBody           map[string]any
}

func (f *fixture) CurrentPrincipal(_ context.Context, h http.Header) (weknora.Principal, error) {
	if !f.active {
		return weknora.Principal{}, fail(401, "auth.revoked")
	}
	if !f.member {
		return weknora.Principal{}, fail(403, "workspace.denied")
	}
	id := strings.TrimPrefix(h.Get("Authorization"), "Bearer ")
	return weknora.Principal{User: &weknora.User{ID: id, IsActive: true}, Tenant: &weknora.Tenant{ID: 1}, Memberships: []weknora.Membership{{TenantID: 1, Role: f.role}}}, nil
}
func (f *fixture) NativeRequest(_ context.Context, method, path string, _ url.Values, _ http.Header, input any) (json.RawMessage, error) {
	var data any
	switch {
	case path == "/api/v1/embed-channels/c":
		data = f.channel
	case path == "/api/v1/agents/a/embed-channels":
		data = map[string]any{"id": "c", "tenant_id": 1, "agent_id": "a", "publish_token": "MUST_NOT_LEAK", "webhook_secret": "MUST_NOT_LEAK"}
	case path == "/api/v1/agents/a":
		data = map[string]any{"id": "a", "tenant_id": 1, "config": map[string]any{"agent_mode": "quick-answer", "kb_selection_mode": "selected", "knowledge_bases": []string{"kb"}}}
	case path == "/api/v1/knowledge-bases/kb":
		if !f.kb {
			return nil, &weknora.Error{StatusCode: 403, Code: "kb.denied"}
		}
		data = map[string]any{"id": "kb"}
	case path == "/api/v1/sessions" && method == "POST":
		data = map[string]any{"id": "s"}
	case strings.HasPrefix(path, "/api/v1/models/"):
		data = map[string]any{"id": strings.TrimPrefix(path, "/api/v1/models/"), "type": "KnowledgeQA", "status": "active"}
	default:
		return nil, fail(404, "fixture.missing")
	}
	raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
	return raw, nil
}
func (f *fixture) KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error) {
	return "kb", nil
}
func (f *fixture) KnowledgeBaseForChunk(_ context.Context, id string, _ http.Header) (string, error) {
	if id == "chunk" {
		return "kb", nil
	}
	return "other", nil
}
func (f *fixture) ValidateSession(context.Context, string, http.Header) error { return nil }
func (f *fixture) Bind(_ context.Context, id string, a nativeaccess.Actor, ids []string) error {
	b := f.native[id]
	if b.Actor != a {
		return fail(403, "binding.denied")
	}
	b.KBIDs = ids
	f.native[id] = b
	return nil
}
func (f *fixture) Binding(_ context.Context, id string) (nativeaccess.Binding, error) {
	b, ok := f.native[id]
	if !ok {
		return b, fail(403, "session.unbound")
	}
	return b, nil
}
func (f *fixture) Bindings(context.Context, nativeaccess.Actor) ([]nativeaccess.Binding, error) {
	return nil, nil
}
func (f *fixture) Record(context.Context, nativeaccess.Actor, string, string, string, []string, string) error {
	return nil
}
func (f *fixture) Create(_ context.Context, b Binding, a nativeaccess.Actor) error {
	f.bindings[b.SessionID] = b
	f.native[b.SessionID] = nativeaccess.Binding{SessionID: b.SessionID, Actor: a}
	return nil
}
func (f *fixture) Get(_ context.Context, id string) (Binding, bool, error) {
	b, ok := f.bindings[id]
	return b, ok, nil
}
func (f *fixture) List(_ context.Context, c string, a nativeaccess.Actor) ([]Binding, error) {
	out := []Binding{}
	for id, b := range f.bindings {
		if b.ChannelID == c && f.native[id].Actor == a {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fixture) Take(context.Context, string, string, int, int) error {
	if f.budget == 0 {
		return fail(429, "assistant.rate_limited")
	}
	f.budget--
	return nil
}
func setup() (*Service, *fixture, http.Handler) {
	f := &fixture{channel: Channel{ID: "c", TenantID: 1, AgentID: "a", Name: "Assistant", Enabled: true, AllowedOrigins: []string{"https://portal.example"}, RateLimitPerMinute: 30, RateLimitPerDay: 1000}, role: "owner", active: true, member: true, kb: true, native: map[string]nativeaccess.Binding{}, bindings: map[string]Binding{}, budget: 100}
	scopes := &nativeaccess.ScopeService{Upstream: f, Store: f}
	s := &Service{Enabled: true, Origin: "https://mindcreek.example", Principals: f, Scopes: scopes, Models: &nativeaccess.ModelPolicy{Upstream: f}, Store: f}
	scopes.SessionGuard = s.GuardSession
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls++
		if r.Method == "POST" && strings.Contains(r.URL.Path, "chat/") {
			_ = json.NewDecoder(r.Body).Decode(&f.lastBody)
		}
		reply(w, 200, map[string]any{"content": "allowed"})
	})
	return s, f, s.Handler(proxy)
}
func request(h http.Handler, method, path, body, user, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, Prefix+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+user)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Tenant-ID", "1")
	r.Header.Set("X-MindCreek-Host-Origin", origin)
	r.Header.Set("X-MindCreek-Session-ID", "s")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func create(t *testing.T, h http.Handler) {
	t.Helper()
	w := request(h, "POST", "1/c/sessions", `{"title":"test"}`, "alice", "https://portal.example")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body)
	}
}
func TestEmployeeFlowUsesRealIdentityAndServerAgent(t *testing.T) {
	_, f, h := setup()
	f.role = "viewer"
	create(t, h)
	w := request(h, "POST", "1/c/proxy/api/v1/knowledge-chat/s", `{"query":"hello"}`, "alice", "https://portal.example")
	if w.Code != 200 || f.calls != 1 {
		t.Fatalf("chat %d %s", w.Code, w.Body)
	}
	if f.lastBody["agent_id"] != "a" || f.native["s"].Actor.ID != "alice" || len(f.native["s"].KBIDs) != 1 {
		t.Fatalf("binding/body %#v %#v", f.native, f.lastBody)
	}
	w = request(h, "GET", "1/c/proxy/api/v1/chunks/by-id/chunk", "", "alice", "https://portal.example")
	if w.Code != 200 {
		t.Fatalf("citation %d %s", w.Code, w.Body)
	}
}
func TestDenialsNeverReachChatOrRetrieval(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		change                   func(*Service, *fixture)
		path, body, user, origin string
	}{
		{name: "employee logout", change: func(_ *Service, f *fixture) { f.active = false }},
		{name: "member removed", change: func(_ *Service, f *fixture) { f.member = false }},
		{name: "channel disabled", change: func(_ *Service, f *fixture) { f.channel.Enabled = false }},
		{name: "feature disabled", change: func(s *Service, _ *fixture) { s.Enabled = false }},
		{name: "KB revoked", change: func(_ *Service, f *fixture) { f.kb = false }},
		{name: "channel moved", change: func(_ *Service, f *fixture) { f.channel.TenantID = 2 }},
		{name: "wrong employee", user: "bob"},
		{name: "wrong origin", origin: "https://evil.example"},
		{name: "missing origin", origin: "missing"},
		{name: "wrong workspace", path: "2/c/proxy/api/v1/knowledge-chat/s"},
		{name: "client agent override", body: `{"query":"q","agent_id":"b"}`},
		{name: "client KB override", body: `{"query":"q","knowledge_base_ids":["other"]}`},
		{name: "query Agent override", path: "1/c/proxy/api/v1/knowledge-chat/s?agent_id=b"},
		{name: "public file mode", path: "1/c/proxy/api/v1/knowledge-chat/s?resource_urls=public"},
		{name: "wrong Agent mode", path: "1/c/proxy/api/v1/agent-chat/s"},
		{name: "local admin cannot consume", change: func(s *Service, _ *fixture) {
			s.Employee = func(context.Context, http.Header) error { return fail(403, "assistant.employee_required") }
		}},
		{name: "upload disabled", body: `{"query":"q","attachment_uploads":[{"data":"x"}]}`},
		{name: "unknown route", path: "1/c/proxy/api/v1/models"},
		{name: "unbound session", path: "1/c/proxy/api/v1/knowledge-chat/other"},
		{name: "rate limit", change: func(_ *Service, f *fixture) { f.budget = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, h := setup()
			create(t, h)
			if tc.change != nil {
				tc.change(s, f)
			}
			if tc.path == "" {
				tc.path = "1/c/proxy/api/v1/knowledge-chat/s"
			}
			if tc.body == "" {
				tc.body = `{"query":"q"}`
			}
			if tc.user == "" {
				tc.user = "alice"
			}
			if tc.origin == "" {
				tc.origin = "https://portal.example"
			}
			if tc.origin == "missing" {
				tc.origin = ""
			}
			w := request(h, "POST", tc.path, tc.body, tc.user, tc.origin)
			if w.Code < 400 || f.calls != 0 {
				t.Fatalf("denial %d calls=%d %s", w.Code, f.calls, w.Body)
			}
		})
	}
}
func TestSessionCannotBypassChannelAndRevocation(t *testing.T) {
	s, f, h := setup()
	create(t, h)
	actor := f.native["s"].Actor
	if _, err := s.Scopes.CheckSession(context.Background(), "s", actor, http.Header{}); err == nil {
		t.Fatal("main API bypass")
	}
	for _, path := range []string{"1/c/proxy/api/v1/messages/s/load", "1/c/proxy/api/v1/sessions/continue-stream/s", "1/c/proxy/api/v1/chunks/by-id/chunk"} {
		f.channel.Enabled = false
		w := request(h, "GET", path, "", "alice", "https://portal.example")
		if w.Code != 403 || f.calls != 0 {
			t.Fatalf("revoked %s: %d", path, w.Code)
		}
	}
}
func TestManagementNativeRoleAndNoSecrets(t *testing.T) {
	_, f, h := setup()
	body := `{"name":"test","allowed_origins":["https://portal.example"],"rate_limit_per_minute":30,"rate_limit_per_day":1000}`
	for _, role := range []string{"owner", "admin", "contributor", "viewer"} {
		f.role = role
		w := request(h, "POST", "agents/a/channels", body, "alice", "")
		if role == "owner" || role == "admin" {
			if w.Code != 201 || strings.Contains(w.Body.String(), "MUST_NOT_LEAK") {
				t.Fatalf("management %s %d %s", role, w.Code, w.Body)
			}
		} else if w.Code != 403 {
			t.Fatalf("role %s: %d", role, w.Code)
		}
	}
	f.role = "owner"
	w := request(h, "POST", "agents/a/channels", strings.ReplaceAll(body, "https://portal.example", "*"), "alice", "")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestMachineCredentialsRejected(t *testing.T) {
	_, _, h := setup()
	r := httptest.NewRequest("GET", Prefix+"1/c/config", nil)
	r.Header.Set("X-API-Key", "machine")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestOriginsAndFramePolicy(t *testing.T) {
	for _, origin := range []string{"*", "https://*.example", "https://x/path", "https://x?q=y", "https://x#y", "https://user:pass@x", "http://intranet", "https://x; frame-ancestors *", "https://x\nX-Test: yes", "null"} {
		if ValidOrigin(origin) {
			t.Errorf("accepted %q", origin)
		}
	}
	s, _, _ := setup()
	r := httptest.NewRequest("GET", "/policy", nil)
	r.Header.Set("X-Original-URI", "/assistant/1/c?host_origin=https%3A%2F%2Fportal.example")
	w := httptest.NewRecorder()
	s.FramePolicy(w, r)
	if w.Code != 204 || w.Header().Get("Content-Security-Policy") != "frame-ancestors 'self' https://portal.example; object-src 'none'; base-uri 'self'" {
		t.Fatal(w.Code, w.Header())
	}
	s.Enabled = false
	w = httptest.NewRecorder()
	s.FramePolicy(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
