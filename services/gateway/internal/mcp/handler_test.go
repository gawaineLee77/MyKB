package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type principalResolverStub struct{}

func (principalResolverStub) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return weknora.Principal{User: &weknora.User{ID: "alice"}, Tenant: &weknora.Tenant{ID: 42}}, nil
}

type principalResolverCapture struct{ headers http.Header }

func (s *principalResolverCapture) CurrentPrincipal(_ context.Context, headers http.Header) (weknora.Principal, error) {
	s.headers = headers.Clone()
	return weknora.Principal{User: &weknora.User{ID: "alice"}, Tenant: &weknora.Tenant{ID: 42}}, nil
}

type toolCallerStub struct {
	name string
}

func (s *toolCallerStub) Call(_ context.Context, name string, _ json.RawMessage, _ authorization.Principal, _ http.Header, _ string) (any, error) {
	s.name = name
	return map[string]any{"ok": true}, nil
}

type limiterStub bool

func (l limiterStub) Allow(string, time.Time) bool { return bool(l) }

func TestModernDiscoveryAndToolCall(t *testing.T) {
	caller := &toolCallerStub{}
	handler, err := NewHandler(principalResolverStub{}, caller, limiterStub(true), "0.5.0-phase4")
	if err != nil {
		t.Fatal(err)
	}
	discover := modernRequest(t, "server/discover", "", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, discover)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"supportedVersions":["2026-07-28"]`) || !strings.Contains(response.Body.String(), `"cacheScope":"private"`) {
		t.Fatalf("discovery response %d: %s", response.Code, response.Body.String())
	}

	call := modernRequest(t, "tools/call", "search_knowledge", `{"name":"search_knowledge","arguments":{"query":"river"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, call)
	if response.Code != http.StatusOK || caller.name != "search_knowledge" || !strings.Contains(response.Body.String(), `"structuredContent":{"ok":true}`) {
		t.Fatalf("tool response %d: %s, call=%s", response.Code, response.Body.String(), caller.name)
	}
}

func TestModernTransportRejectsAnonymousAndHeaderMismatch(t *testing.T) {
	handler, _ := NewHandler(principalResolverStub{}, &toolCallerStub{}, limiterStub(true), "test")
	request := modernRequest(t, "tools/list", "", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	request.Header.Del("Authorization")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", response.Code)
	}

	request = modernRequest(t, "tools/list", "", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	request.Header.Set("Mcp-Method", "tools/call")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "-32020") {
		t.Fatalf("mismatch response %d: %s", response.Code, response.Body.String())
	}
}

func TestLegacyInitializeAndRateLimit(t *testing.T) {
	handler, _ := NewHandler(principalResolverStub{}, &toolCallerStub{}, limiterStub(true), "test")
	request := httptest.NewRequest(http.MethodPost, "http://mindcreek.local/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"protocolVersion":"2025-11-25"`) {
		t.Fatalf("initialize response %d: %s", response.Code, response.Body.String())
	}

	limited, _ := NewHandler(principalResolverStub{}, &toolCallerStub{}, limiterStub(false), "test")
	response = httptest.NewRecorder()
	limited.ServeHTTP(response, request.Clone(context.Background()))
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d", response.Code)
	}
}

func TestToolDefinitionsAreDeterministicAndReadOnly(t *testing.T) {
	definitions := toolDefinitions()
	want := []string{"ask_knowledge_agent", "get_source_excerpt", "list_knowledge_bases", "list_publications", "list_subscriptions", "search_knowledge"}
	if len(definitions) != len(want) {
		t.Fatalf("tool count = %d", len(definitions))
	}
	for index, name := range want {
		if definitions[index]["name"] != name {
			t.Fatalf("tool %d = %v, want %s", index, definitions[index]["name"], name)
		}
		annotations := definitions[index]["annotations"].(map[string]bool)
		if !annotations["readOnlyHint"] || annotations["destructiveHint"] {
			t.Fatalf("tool %s annotations = %+v", name, annotations)
		}
	}
}

func TestTransportRejectsTrailingJSON(t *testing.T) {
	handler, _ := NewHandler(principalResolverStub{}, &toolCallerStub{}, limiterStub(true), "test")
	request := httptest.NewRequest(http.MethodPost, "http://mindcreek.local/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}} {}`))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "-32700") {
		t.Fatalf("trailing JSON response %d: %s", response.Code, response.Body.String())
	}
}

func TestTransportRejectsCrossOriginWorkspaceAndOversizedPayload(t *testing.T) {
	principals := &principalResolverCapture{}
	handler, _ := NewHandler(principals, &toolCallerStub{}, limiterStub(true), "test")

	crossOrigin := httptest.NewRequest(http.MethodPost, "http://mindcreek.local/mcp", strings.NewReader(`{}`))
	crossOrigin.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, crossOrigin)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", response.Code)
	}

	wrongWorkspace := httptest.NewRequest(http.MethodPost, "http://mindcreek.local/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`))
	wrongWorkspace.Header.Set("Authorization", "Bearer token")
	wrongWorkspace.Header.Set("Content-Type", "application/json")
	wrongWorkspace.Header.Set("X-Tenant-ID", "99")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, wrongWorkspace)
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong-workspace status = %d", response.Code)
	}
	if principals.headers.Get("X-Tenant-ID") != "" {
		t.Fatal("workspace override influenced principal resolution")
	}

	oversized := httptest.NewRequest(http.MethodPost, "http://mindcreek.local/mcp", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	oversized.Header.Set("Authorization", "Bearer token")
	oversized.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, oversized)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", response.Code)
	}
}

func modernRequest(t *testing.T, method, name, params string) *http.Request {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + params + `}`
	request := httptest.NewRequest(http.MethodPost, "http://mindcreek.local/mcp", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", ModernProtocol)
	request.Header.Set("Mcp-Method", method)
	if name != "" {
		request.Header.Set("Mcp-Name", name)
	}
	return request
}
