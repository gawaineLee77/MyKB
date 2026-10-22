package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type wikiFixture struct {
	NativeKnowledge
	nativeaccess.NativeReader
	page                        weknora.WikiPage
	allowed                     map[string]bool
	wikiEnabled                 map[string]bool
	parents                     map[string]string
	contentCalls, metadataCalls int
	afterRead                   func()
	headers                     []http.Header
	queries                     []string
}

func (f *wikiFixture) WikiEnabled(_ context.Context, id string, h http.Header) (bool, error) {
	f.metadataCalls++
	f.headers = append(f.headers, h.Clone())
	return f.wikiEnabled[id], nil
}
func (f *wikiFixture) SearchWikiPages(_ context.Context, kb, query string, limit int, h http.Header) ([]weknora.WikiPage, error) {
	f.contentCalls++
	f.queries = append(f.queries, query)
	f.headers = append(f.headers, h.Clone())
	if f.afterRead != nil {
		f.afterRead()
	}
	return []weknora.WikiPage{f.page}, nil
}
func (f *wikiFixture) GetWikiPage(_ context.Context, kb, slug string, h http.Header) (weknora.WikiPage, error) {
	f.contentCalls++
	f.headers = append(f.headers, h.Clone())
	if f.afterRead != nil {
		f.afterRead()
	}
	return f.page, nil
}
func (f *wikiFixture) NativeRequest(_ context.Context, method, path string, _ url.Values, h http.Header, _ any) (json.RawMessage, error) {
	f.headers = append(f.headers, h.Clone())
	if path != "/api/v1/knowledge-bases" {
		return nil, errors.New("unexpected native path")
	}
	return json.RawMessage(`{"success":true,"data":[{"id":"private"},{"id":"kb"},{"id":"plain"}]}`), nil
}
func (f *wikiFixture) KnowledgeBaseForKnowledge(_ context.Context, id string, h http.Header) (string, error) {
	f.headers = append(f.headers, h.Clone())
	return f.parents[id], nil
}
func (f *wikiFixture) KnowledgeBaseForChunk(_ context.Context, id string, h http.Header) (string, error) {
	f.headers = append(f.headers, h.Clone())
	return f.parents[id], nil
}

type wikiAudit struct {
	nativeaccess.SessionStore
	actor              nativeaccess.Actor
	operation, outcome string
	ids                []string
	err                error
}

func (a *wikiAudit) Record(_ context.Context, actor nativeaccess.Actor, operation, outcome, _ string, ids []string, _ string) error {
	a.actor = actor
	a.operation = operation
	a.outcome = outcome
	a.ids = ids
	return a.err
}

func wikiServiceFixture() (*NativeService, *wikiFixture, *wikiAudit) {
	f := &wikiFixture{
		page:    weknora.WikiPage{ID: "page", TenantID: 42, KnowledgeBaseID: "kb", Slug: "设备/水泵", Title: "Synthetic pump", PageType: "entity", Status: "published", Summary: "Synthetic summary", Content: "检查水泵，然后维护。", SourceRefs: []string{"doc|Synthetic"}, ChunkRefs: []string{"chunk"}},
		allowed: map[string]bool{"kb": true, "plain": true}, wikiEnabled: map[string]bool{"kb": true}, parents: map[string]string{"doc": "kb", "chunk": "kb"},
	}
	audit := &wikiAudit{}
	s := &NativeService{Knowledge: f, Scopes: &nativeaccess.ScopeService{Upstream: f, Store: audit, KnowledgeAccess: func(_ context.Context, id string, h http.Header) error {
		f.headers = append(f.headers, h.Clone())
		if !f.allowed[id] {
			return &nativeaccess.Error{Status: 404, Code: "kb_acl.not_found"}
		}
		return nil
	}}}
	return s, f, audit
}

var wikiActor = nativeaccess.Actor{TenantID: 42, Kind: "api_key", ID: "synthetic-fingerprint"}
var wikiHeaders = http.Header{"X-Api-Key": {"synthetic-retrieve-key"}, "X-Tenant-Id": {"42"}}

func callWikiTest(s *NativeService, name, raw string) (any, error) {
	return s.CallNative(context.Background(), name, json.RawMessage(raw), wikiActor, wikiHeaders, "synthetic-correlation")
}

func TestWikiMCPDefaultScopeOriginalCredentialAndAudit(t *testing.T) {
	s, f, audit := wikiServiceFixture()
	value, err := callWikiTest(s, "wiki_search", `{"query":"水泵"}`)
	if err != nil {
		t.Fatal(err)
	}
	result := value.(map[string]any)
	if !reflect.DeepEqual(result["effective_scope"], []string{"kb", "plain"}) || !reflect.DeepEqual(result["searched_knowledge_base_ids"], []string{"kb"}) || f.contentCalls != 1 {
		t.Fatal(result, f.contentCalls)
	}
	hits := result["results"].([]wikiHit)
	if len(hits) != 1 || hits[0].Slug != "设备/水泵" || !strings.Contains(hits[0].Snippet, "水泵") {
		t.Fatal(hits)
	}
	for _, h := range f.headers {
		if h.Get("X-API-Key") != wikiHeaders.Get("X-API-Key") || h.Get("Authorization") != "" || h.Get("X-Tenant-ID") != "42" {
			t.Fatal("broadened or missing credential", h)
		}
	}
	if audit.operation != "mcp.wiki_search" || audit.actor != wikiActor || audit.outcome != "allowed" || !reflect.DeepEqual(audit.ids, []string{"kb", "plain"}) {
		t.Fatal(audit)
	}
}

func TestWikiMCPRejectsEntireExplicitScopeBeforeRetrieval(t *testing.T) {
	s, f, audit := wikiServiceFixture()
	value, err := callWikiTest(s, "wiki_search", `{"query":"test","knowledge_base_ids":["kb","private"]}`)
	if err != ErrDenied || value != nil || f.contentCalls != 0 || f.metadataCalls != 0 || audit.outcome != "denied" {
		t.Fatal(value, err, f.contentCalls, f.metadataCalls, audit)
	}
	value, err = callWikiTest(s, "wiki_read_page", `{"knowledge_base_id":"private","slug":"page"}`)
	if err != ErrDenied || value != nil || f.contentCalls != 0 {
		t.Fatal(value, err, f.contentCalls)
	}
}

func TestWikiMCPOmittedScopeDoesNotConflateEmptyOrNull(t *testing.T) {
	for _, raw := range []string{`{"query":"水泵","knowledge_base_ids":[]}`, `{"query":"水泵","knowledge_base_ids":null}`} {
		s, f, audit := wikiServiceFixture()
		result, err := callWikiTest(s, "wiki_search", raw)
		if result != nil || err != ErrDenied || f.contentCalls != 0 || f.metadataCalls != 0 || len(f.headers) != 0 || audit.outcome != "denied" {
			t.Fatal("explicit empty scope expanded", raw, result, err, f.contentCalls, f.metadataCalls, f.headers)
		}
	}
	s, f, _ := wikiServiceFixture()
	if result, err := callWikiTest(s, "wiki_search", `{"query":"水泵"}`); result == nil || err != nil || f.contentCalls != 1 {
		t.Fatal("omitted default scope failed", result, err, f.contentCalls)
	}
}

func TestWikiMCPRechecksPermissionAfterFetchAndNextCall(t *testing.T) {
	for _, call := range []struct{ name, raw string }{{"wiki_search", `{"query":"x","knowledge_base_ids":["kb"]}`}, {"wiki_read_page", `{"knowledge_base_id":"kb","slug":"设备/水泵"}`}} {
		s, f, _ := wikiServiceFixture()
		f.afterRead = func() { f.allowed["kb"] = false }
		if result, err := callWikiTest(s, call.name, call.raw); result != nil || err != ErrDenied {
			t.Fatal(result, err)
		}
		if f.contentCalls != 1 {
			t.Fatal(f.contentCalls)
		}
		if result, err := callWikiTest(s, call.name, call.raw); result != nil || err != ErrDenied || f.contentCalls != 1 {
			t.Fatal("revoked next call retrieved", result, err, f.contentCalls)
		}
	}
}

func TestWikiMCPValidatesResponseTenantParentSlugAndReferences(t *testing.T) {
	for name, mutate := range map[string]func(*wikiFixture){
		"tenant":         func(f *wikiFixture) { f.page.TenantID = 99 },
		"KB":             func(f *wikiFixture) { f.page.KnowledgeBaseID = "private" },
		"slug":           func(f *wikiFixture) { f.page.Slug = "other" },
		"source":         func(f *wikiFixture) { f.parents["doc"] = "private" },
		"chunk":          func(f *wikiFixture) { f.parents["chunk"] = "private" },
		"invalid source": func(f *wikiFixture) { f.page.SourceRefs = []string{"../secret|title"} },
		"unbounded refs": func(f *wikiFixture) { f.page.ChunkRefs = make([]string, 257) },
	} {
		t.Run(name, func(t *testing.T) {
			s, f, _ := wikiServiceFixture()
			mutate(f)
			if result, err := callWikiTest(s, "wiki_read_page", `{"knowledge_base_id":"kb","slug":"设备/水泵"}`); result != nil || err != ErrDenied {
				t.Fatal(result, err)
			}
		})
	}
}

func TestWikiMCPReadPaginationAndResourceRedaction(t *testing.T) {
	s, f, _ := wikiServiceFixture()
	f.page.Content = "中文文本 https://example.test/help ![private](https://storage.test/x?X-Amz-Signature=secret) resource://private/image minio://bucket/key /api/v1/knowledge/private/download"
	var joined strings.Builder
	offset := 0
	for {
		raw, _ := json.Marshal(map[string]any{"knowledge_base_id": "kb", "slug": f.page.Slug, "offset": offset, "max_chars": 12})
		value, err := callWikiTest(s, "wiki_read_page", string(raw))
		if err != nil {
			t.Fatal(err)
		}
		result := value.(map[string]any)
		content := result["content"].(string)
		if !utf8.ValidString(content) || utf8.RuneCountInString(content) > 12 {
			t.Fatal(content)
		}
		joined.WriteString(content)
		if !result["truncated"].(bool) {
			break
		}
		next := result["next_offset"].(int)
		if next <= offset {
			t.Fatal(result)
		}
		offset = next
	}
	text := joined.String()
	if !strings.Contains(text, "https://example.test/help") || !strings.Contains(text, "中文文本") {
		t.Fatal(text)
	}
	for _, secret := range []string{"secret", "storage.test", "resource://", "minio://", "/api/v1/knowledge/"} {
		if strings.Contains(text, secret) {
			t.Fatal("resource leaked", text)
		}
	}
}

func TestWikiTextRedactsCloudSASTokensEscapesAndPrivateRoutes(t *testing.T) {
	for _, link := range []string{
		"https://blob.test/file?sv=2025&sig=synthetic-secret&se=soon",
		"https://blob.test/file?sv=2025&amp;sig=synthetic-secret",
		"https://blob.test/file?%73%69%67=synthetic-secret",
		"https://blob.test/file?%2573%2569%2567=synthetic-secret",
		"https://blob.test/file?X-Goog-Credential=synthetic-secret",
		"https://blob.test/file?Authorization=synthetic-secret",
		"https://blob.test/file?access_token=synthetic-secret",
		"https://synthetic-secret@blob.test/file",
		"https://product.test/%61pi/v1/resources/synthetic-secret",
		"https://product.test/uploads/synthetic-secret",
		"resource%3A%2F%2Fsynthetic-secret",
	} {
		if clean := wikiSafeText("Read " + link); strings.Contains(clean, "synthetic-secret") || !strings.Contains(clean, "[resource redacted]") {
			t.Fatalf("%q => %q", link, clean)
		}
	}
	for _, link := range []string{"https://example.test/docs", "https://example.test/help?q=water&lang=zh"} {
		if clean := wikiSafeText(link); clean != link {
			t.Fatalf("public link changed: %q => %q", link, clean)
		}
	}
}

func TestWikiMCPRejectsInvalidArgumentsAndMissingACL(t *testing.T) {
	for _, call := range []struct{ name, raw string }{
		{"wiki_search", `{"query":" "}`}, {"wiki_search", `{"query":"x","limit":21}`}, {"wiki_search", `{"query":"x","agent_id":"broader-agent"}`}, {"wiki_search", `{"query":"x","knowledge_base_ids":["../private"]}`},
		{"wiki_search", `{"query":"x","limit":0}`}, {"wiki_read_page", `{"knowledge_base_id":"kb","slug":"page","max_chars":0}`},
		{"wiki_read_page", `{"knowledge_base_id":"kb","slug":"a/../b"}`}, {"wiki_read_page", `{"knowledge_base_id":"kb","slug":"page","max_chars":20001}`}, {"wiki_read_page", `{"knowledge_base_id":"kb","slug":"page","offset":-1}`}, {"wiki_read_page", `{"knowledge_base_id":"kb","slug":"page","tenant_id":99}`},
	} {
		s, f, _ := wikiServiceFixture()
		result, err := callWikiTest(s, call.name, call.raw)
		if result != nil || err != ErrInvalid || f.contentCalls != 0 {
			t.Fatal(call, result, err, f.contentCalls)
		}
	}
	s, f, _ := wikiServiceFixture()
	s.Scopes.KnowledgeAccess = nil
	if _, err := callWikiTest(s, "wiki_search", `{"query":"x"}`); err != ErrUnavailable || f.contentCalls != 0 {
		t.Fatal(err)
	}
}

func TestWikiMCPAuditFailureDiscardsContent(t *testing.T) {
	s, _, audit := wikiServiceFixture()
	audit.err = errors.New("synthetic audit unavailable")
	if result, err := callWikiTest(s, "wiki_read_page", `{"knowledge_base_id":"kb","slug":"设备/水泵"}`); result != nil || err != ErrUnavailable {
		t.Fatal(result, err)
	}
}

func TestWikiMCPMetadataAndLegacyCompatibility(t *testing.T) {
	caller := &nativeCallerCapture{}
	h, _ := NewNativeHandler(nativeResolverStub{}, caller, limiterStub(true), "test")
	for _, modern := range []bool{true, false} {
		result := h.toolList(modern)
		found := map[string]bool{}
		for _, tool := range result["tools"].([]map[string]any) {
			name := tool["name"].(string)
			if !strings.HasPrefix(name, "wiki_") {
				continue
			}
			found[name] = true
			annotations := tool["annotations"].(map[string]bool)
			if !annotations["readOnlyHint"] || !annotations["idempotentHint"] || annotations["destructiveHint"] || annotations["openWorldHint"] {
				t.Fatal(tool)
			}
			if tool["inputSchema"].(map[string]any)["additionalProperties"] != false {
				t.Fatal(tool)
			}
		}
		if !found["wiki_search"] || !found["wiki_read_page"] {
			t.Fatal(result)
		}
	}
	legacy, _ := NewHandler(principalResolverStub{}, &toolCallerStub{}, limiterStub(true), "test")
	for _, tool := range legacy.toolList(false)["tools"].([]map[string]any) {
		if strings.HasPrefix(tool["name"].(string), "wiki_") {
			t.Fatal("legacy domain service advertises unsupported Wiki")
		}
	}
	r := modernRequest(t, "tools/call", "wiki_delete_page", `{"name":"wiki_delete_page","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"code":-32602`) || len(caller.actors) != 0 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal(w.Body.String())
	}
}

func TestWikiMCPTransportUsesMachineIdentityAndCurrentAuthorization(t *testing.T) {
	s, f, audit := wikiServiceFixture()
	h, err := NewNativeHandler(nativeResolverStub{}, s, limiterStub(true), "test")
	if err != nil {
		t.Fatal(err)
	}
	request := func() *http.Request {
		r := modernRequest(t, "tools/call", "wiki_read_page", `{"name":"wiki_read_page","arguments":{"knowledge_base_id":"kb","slug":"设备/水泵"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`)
		r.Header.Del("Authorization")
		r.Header.Set("X-API-Key", "synthetic-retrieve-key")
		r.Header.Set("X-Tenant-ID", "42")
		return r
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request())
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"structuredContent"`) || !strings.Contains(w.Body.String(), "检查水泵") || audit.actor.Kind != "api_key" || audit.actor.ID == "native-owner" || strings.Contains(audit.actor.ID, "synthetic") {
		t.Fatal(w.Body.String(), audit.actor)
	}
	f.allowed["kb"] = false
	w = httptest.NewRecorder()
	h.ServeHTTP(w, request())
	if !strings.Contains(w.Body.String(), `"code":-32004`) || strings.Contains(w.Body.String(), "检查水泵") || f.contentCalls != 1 {
		t.Fatal(w.Body.String(), f.contentCalls)
	}
}
