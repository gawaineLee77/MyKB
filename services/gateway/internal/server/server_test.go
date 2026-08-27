package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/policy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/space"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	u, err := url.Parse("http://upstream.invalid:8080")
	if err != nil {
		t.Fatal(err)
	}
	return config.Config{
		ListenAddr:      ":8080",
		ProductVersion:  "test-version",
		UpstreamURL:     u,
		UpstreamVersion: "v0.7.2",
		UpstreamTimeout: time.Second,
	}
}

func TestSkeletonHealthAndVersion(t *testing.T) {
	handler := NewSkeleton(testConfig(t))
	for _, tc := range []struct {
		path string
		key  string
		want string
	}{
		{path: "/health", key: "status", want: "ok"},
		{path: "/version", key: "version", want: "test-version"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", tc.path, recorder.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s invalid JSON: %v", tc.path, err)
		}
		if body[tc.key] != tc.want {
			t.Fatalf("GET %s %s = %q", tc.path, tc.key, body[tc.key])
		}
	}
}

func TestSkeletonStructuredNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewSkeleton(testConfig(t)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "route.not_found" || body.Error.RequestID == "" {
		t.Fatalf("unexpected error: %+v", body.Error)
	}
}

func TestGatewayProxiesUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/config" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		if r.Header.Get("X-Request-ID") == "" {
			t.Fatal("gateway did not add X-Request-ID")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()

	cfg := testConfig(t)
	cfg.UpstreamURL, _ = url.Parse(upstream.URL)
	recorder := httptest.NewRecorder()
	NewGateway(cfg, nil, nil, Dependencies{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/config", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"success":true}` {
		t.Fatalf("proxy response status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestCapabilityEndpointUsesRegistry(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "capabilities.json")
	payload := `{"schema_version":1,"phase":"phase1","capabilities":{"im":false,"miniprogram":false,"cli":false,"embed":false,"browser_extension":false,"web_search":false,"mcp":false,"asr":false,"data_analysis":false,"external_connectors":false,"kb_personal_notes":true,"rag_plain":true,"rag_graph":false,"rag_pixel":false,"ontology":false}}`
	if err := os.WriteFile(filename, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := capability.Load(filename)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	NewGateway(testConfig(t), registry, nil, Dependencies{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities/knowledge-modes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var document capability.Document
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Capabilities) != len(capability.Keys()) {
		t.Fatalf("flags=%d, want=%d", len(document.Capabilities), len(capability.Keys()))
	}
	for _, key := range capability.Keys() {
		if document.Capabilities[key] != registry.Capabilities[key] {
			t.Fatalf("API flag %q differs from registry", key)
		}
	}
}

func TestRoutePolicyDeniesDisabledAndUnknownRoutes(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	filename := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../config/phase1-route-policy.json"))
	routePolicy, err := policy.Load(filename, "v0.7.2")
	if err != nil {
		t.Fatal(err)
	}

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()
	cfg := testConfig(t)
	cfg.UpstreamURL, _ = url.Parse(upstream.URL)
	handler := NewGateway(cfg, nil, routePolicy, Dependencies{})

	for _, requestPath := range []string{"/api/v1/im/channels", "/api/v1/agents/agent-1/im-channels", "/r/token", "/api/v1/mcp-services"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
		assertErrorCode(t, recorder, http.StatusNotFound, "feature.disabled")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/not-in-contract", nil))
	assertErrorCode(t, recorder, http.StatusNotFound, "route.unclassified")
	if upstreamCalls.Load() != 0 {
		t.Fatalf("denied requests reached upstream %d times", upstreamCalls.Load())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/config", nil))
	if recorder.Code != http.StatusOK || upstreamCalls.Load() != 1 {
		t.Fatalf("pass-through status=%d upstreamCalls=%d", recorder.Code, upstreamCalls.Load())
	}
}

type principalResolverFunc func(context.Context, http.Header) (weknora.Principal, error)

func (function principalResolverFunc) CurrentPrincipal(ctx context.Context, headers http.Header) (weknora.Principal, error) {
	return function(ctx, headers)
}

func TestKBControlledRoutesRequireTrustedPrincipal(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	routePolicy, err := policy.Load(filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../config/phase1-route-policy.json")), "v0.7.2")
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()
	cfg := testConfig(t)
	cfg.UpstreamURL, _ = url.Parse(upstream.URL)

	t.Run("missing credential", func(t *testing.T) {
		handler := NewGateway(cfg, nil, routePolicy, Dependencies{Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			t.Fatal("resolver called without credentials")
			return weknora.Principal{}, nil
		})})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-1", nil))
		assertErrorCode(t, recorder, http.StatusUnauthorized, "auth.required")
	})

	t.Run("invalid credential", func(t *testing.T) {
		handler := NewGateway(cfg, nil, routePolicy, Dependencies{Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			return weknora.Principal{}, &weknora.Error{Code: "upstream.unauthorized", StatusCode: http.StatusUnauthorized}
		})})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-1", nil)
		request.Header.Set("Authorization", "Bearer invalid")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		assertErrorCode(t, recorder, http.StatusUnauthorized, "auth.invalid")
	})

	t.Run("cross workspace", func(t *testing.T) {
		handler := NewGateway(cfg, nil, routePolicy, Dependencies{Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			return testPrincipal("alice", 42), nil
		})})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-1", nil)
		request.Header.Set("Authorization", "Bearer valid")
		request.Header.Set("X-Tenant-ID", "99")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		assertErrorCode(t, recorder, http.StatusForbidden, "workspace.denied")
	})
}

type fakeKnowledgeSpaces struct {
	createInput space.CreateInput
	key         string
}

func (f *fakeKnowledgeSpaces) Create(_ context.Context, input space.CreateInput, key string, identity access.Identity, _ http.Header) (space.CreateResult, error) {
	f.createInput = input
	f.key = key
	if identity.UserID != "alice" || identity.TenantID != 42 {
		return space.CreateResult{}, errors.New("unexpected identity")
	}
	return space.CreateResult{
		KnowledgeBaseID: "kb-created", Name: input.Name, ProductMode: profile.ModePersonalNotes,
		IndexProfile: "notes_plain", AccessPolicy: profile.PolicyOwnerOnly, Created: true,
	}, nil
}

func (f *fakeKnowledgeSpaces) GetProfile(_ context.Context, id string, identity access.Identity, _ http.Header) (profile.Profile, error) {
	return profile.Profile{
		UpstreamKBID: id, TenantID: identity.TenantID, OwnerUserID: identity.UserID,
		ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly, SchemaVersion: 1,
	}, nil
}

func TestProductKnowledgeSpaceEndpoints(t *testing.T) {
	spaces := &fakeKnowledgeSpaces{}
	dependencies := Dependencies{
		Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			return testPrincipal("alice", 42), nil
		}),
		Spaces: spaces,
	}
	handler := NewGateway(testConfig(t), nil, nil, dependencies)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-spaces", strings.NewReader(
		`{"mode":"personal_notes","name":"Alice Notes","embedding_model_id":"model-1"}`,
	))
	request.Header.Set("Authorization", "Bearer valid")
	request.Header.Set("Idempotency-Key", "create-note-0001")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || spaces.key != "create-note-0001" || spaces.createInput.Name != "Alice Notes" {
		t.Fatalf("create status=%d key=%q input=%+v body=%s", recorder.Code, spaces.key, spaces.createInput, recorder.Body.String())
	}

	profileRequest := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-created/product-profile", nil)
	profileRequest.Header.Set("Authorization", "Bearer valid")
	profileRecorder := httptest.NewRecorder()
	handler.ServeHTTP(profileRecorder, profileRequest)
	if profileRecorder.Code != http.StatusOK || !strings.Contains(profileRecorder.Body.String(), `"upstream_kb_id":"kb-created"`) {
		t.Fatalf("profile status=%d body=%s", profileRecorder.Code, profileRecorder.Body.String())
	}
}

func testPrincipal(userID string, tenantID uint64) weknora.Principal {
	return weknora.Principal{
		User:   &weknora.User{ID: userID, TenantID: tenantID},
		Tenant: &weknora.Tenant{ID: tenantID, Name: "Test"},
	}
}

func assertErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code=%q, want=%q", body.Error.Code, code)
	}
}
