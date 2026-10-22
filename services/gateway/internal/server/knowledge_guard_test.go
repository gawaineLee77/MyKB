package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/assistant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbguard"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// This fixture keeps native tenant/scope checks permissive so the real server
// composition must supply the additional KB action and response checks.
type guardCompositionFixture struct {
	visible bool
	calls   int
	checks  int
	t       *testing.T
}

func (f *guardCompositionFixture) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return weknora.Principal{User: &weknora.User{ID: "employee", Email: "alias@example.invalid", IsActive: true}, Tenant: &weknora.Tenant{ID: 1}, Memberships: []weknora.Membership{{TenantID: 1, Role: "contributor"}}}, nil
}
func (f *guardCompositionFixture) Authorize(_ context.Context, h http.Header, kb string, actions ...string) (kbacl.Permissions, error) {
	if h.Get("Authorization") != "Bearer human" || h.Get("X-API-Key") != "" {
		f.t.Fatalf("ACL saw execution identity: %v", h)
	}
	if !f.visible || kb != "kb" {
		return kbacl.Permissions{}, &kbguard.Error{Status: 404, Code: "kb_acl.not_found"}
	}
	for _, action := range actions {
		if action != "document.update" {
			return kbacl.Permissions{}, &kbguard.Error{Status: 403, Code: "kb_acl.operation_denied"}
		}
	}
	return kbacl.Permissions{Visible: true, Operations: []string{"document.update"}}, nil
}
func (f *guardCompositionFixture) AllowedKnowledgeBases(context.Context, http.Header, []string) ([]string, error) {
	if f.visible {
		return []string{"kb"}, nil
	}
	return []string{}, nil
}
func (f *guardCompositionFixture) NativeRequest(_ context.Context, _ string, path string, _ url.Values, _ http.Header, _ any) (json.RawMessage, error) {
	var data any
	switch path {
	case "/api/v1/embed-channels/channel":
		data = assistant.Channel{ID: "channel", TenantID: 1, AgentID: "agent", Enabled: true, AllowedOrigins: []string{"https://portal.example"}}
	case "/api/v1/agents/agent":
		data = map[string]any{"id": "agent", "tenant_id": 1, "config": map[string]any{"agent_mode": "quick-answer", "kb_selection_mode": "selected", "knowledge_bases": []string{"kb"}}}
	case "/api/v1/knowledge-bases/kb":
		data = map[string]any{"id": "kb"}
	case "/api/v1/knowledge/doc":
		data = map[string]any{"id": "doc", "knowledge_base_id": "kb"}
	default:
		f.t.Fatalf("unexpected fixture request %s", path)
	}
	raw, err := json.Marshal(map[string]any{"success": true, "data": data})
	return raw, err
}
func (*guardCompositionFixture) KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error) {
	return "kb", nil
}
func (*guardCompositionFixture) KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error) {
	return "kb", nil
}
func (*guardCompositionFixture) ValidateSession(context.Context, string, http.Header) error {
	return nil
}
func (*guardCompositionFixture) Bind(context.Context, string, nativeaccess.Actor, []string) error {
	return nil
}
func (*guardCompositionFixture) Binding(context.Context, string) (nativeaccess.Binding, error) {
	return nativeaccess.Binding{SessionID: "session", Actor: nativeaccess.Actor{Kind: "human", ID: "employee", TenantID: 1}, KBIDs: []string{"kb"}}, nil
}
func (*guardCompositionFixture) Bindings(context.Context, nativeaccess.Actor) ([]nativeaccess.Binding, error) {
	return nil, nil
}
func (*guardCompositionFixture) Record(context.Context, nativeaccess.Actor, string, string, string, []string, string) error {
	return nil
}
func (*guardCompositionFixture) Create(context.Context, assistant.Binding, nativeaccess.Actor) error {
	return nil
}
func (*guardCompositionFixture) Get(context.Context, string) (assistant.Binding, bool, error) {
	return assistant.Binding{SessionID: "session", ChannelID: "channel", AgentID: "agent", HostOrigin: "https://portal.example"}, true, nil
}
func (*guardCompositionFixture) List(context.Context, string, nativeaccess.Actor) ([]assistant.Binding, error) {
	return nil, nil
}
func (*guardCompositionFixture) Take(context.Context, string, string, int, int) error { return nil }

func (f *guardCompositionFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls++
	if r.Method == "PUT" && (r.Header.Get("X-API-Key") != "internal-executor" || r.Header.Get("Authorization") != "") {
		f.t.Fatalf("write did not use fixed execution identity: %v", r.Header)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Request: r, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":"doc","knowledge_base_id":"kb","content":"allowed","file_path":"s3://private/raw.pdf"}}`))}, nil
}

func newGuardComposition(t *testing.T) (http.Handler, *guardCompositionFixture) {
	t.Helper()
	f := &guardCompositionFixture{visible: true, t: t}
	// No network listener is needed. These tests are deliberately nonparallel:
	// ReverseProxy uses DefaultTransport when no custom transport is installed.
	previous := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = previous })
	routes, err := nativeaccess.LoadRoutes("../../../../config/r3-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	scopes := &nativeaccess.ScopeService{Upstream: f, Store: f}
	models := &nativeaccess.ModelPolicy{Upstream: f}
	employee := &assistant.Service{Enabled: true, Principals: f, Scopes: scopes, Models: models, Store: f}
	scopes.SessionGuard = employee.GuardSession
	gate := &nativeaccess.Gate{Routes: routes, Principals: f, ScopeCheck: scopes.Check, ModelCheck: models.Check, ResponseCheck: func(r *http.Response) error {
		f.checks++
		if r.Request.Header.Get("Authorization") != "Bearer human" || r.Request.Header.Get("X-API-Key") != "" {
			t.Fatalf("response scope check saw execution identity: %v", r.Request.Header)
		}
		return nil
	}}
	guard := &kbguard.Guard{ACL: f, Upstream: f, Audit: func(context.Context, http.Header, kbguard.AuditEvent) error { return nil }, WithLock: func(_ context.Context, _ http.Header, _ []string, fn func() error) error { return fn() }, WithCollaborator: func(_ context.Context, _ http.Header, _ string, _ []string, fn func(http.Header) error) error {
		return fn(http.Header{"X-Api-Key": {"internal-executor"}})
	}}
	cfg := testConfig(t)
	cfg.NativeWorkspaceEnabled = true
	now := time.Now()
	store := &enterpriseTestStore{i: enterprise.Installation{Stage: "ready"}, o: enterprise.OnboardingRecord{Subject: "employee-subject", CompletedAt: &now}}
	return NewGateway(cfg, nil, nil, Dependencies{Principals: f, Enterprise: &enterprise.Service{Store: store, Identities: enterpriseTestDirectory{}}, NativeAccess: gate, KBGuard: guard, Assistant: employee}), f
}

func guardCompositionRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer human")
	r.Header.Set("X-Tenant-ID", "1")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-MindCreek-Host-Origin", "https://portal.example")
	r.Header.Set("X-MindCreek-Session-ID", "session")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGatewayAssistantCannotBypassKnowledgeActionGuard(t *testing.T) {
	h, f := newGuardComposition(t)
	const prefix = assistant.Prefix + "1/channel/proxy"
	for _, path := range []string{"/api/v1/knowledge/doc/preview", prefix + "/api/v1/knowledge/doc/preview"} {
		w := guardCompositionRequest(h, "GET", path, "")
		if w.Code != 403 || !strings.Contains(w.Body.String(), "kb_acl.operation_denied") {
			t.Fatalf("%s returned %d %s", path, w.Code, w.Body)
		}
	}
	f.visible = false
	for _, path := range []string{"/api/v1/knowledge/doc", prefix + "/api/v1/knowledge/doc"} {
		w := guardCompositionRequest(h, "GET", path, "")
		if w.Code != 404 || !strings.Contains(w.Body.String(), "kb_acl.not_found") {
			t.Fatalf("%s returned %d %s", path, w.Code, w.Body)
		}
	}
	if f.calls != 0 {
		t.Fatalf("denied content reached upstream %d times", f.calls)
	}
}

func TestGatewayCollaboratorResponseKeepsOriginalIdentity(t *testing.T) {
	h, f := newGuardComposition(t)
	w := guardCompositionRequest(h, "PUT", "/api/v1/knowledge/doc", `{"title":"updated"}`)
	if w.Code != 200 || f.calls != 1 || f.checks != 1 || strings.Contains(w.Body.String(), "s3://") || strings.Contains(w.Body.String(), "file_path") {
		t.Fatalf("status=%d calls=%d checks=%d body=%s", w.Code, f.calls, f.checks, w.Body)
	}
}

func TestGatewayAssistantFiltersAllowedContentResponse(t *testing.T) {
	h, f := newGuardComposition(t)
	w := guardCompositionRequest(h, "GET", assistant.Prefix+"1/channel/proxy/api/v1/knowledge/doc", "")
	if w.Code != 200 || f.calls != 1 || f.checks != 1 || strings.Contains(w.Body.String(), "file_path") || !strings.Contains(w.Body.String(), "allowed") {
		t.Fatalf("status=%d calls=%d checks=%d body=%s", w.Code, f.calls, f.checks, w.Body)
	}
}
