package kbguard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
)

type fakeACL struct {
	permissions map[string]kbacl.Permissions
	calls       []string
}

func (a *fakeACL) Authorize(_ context.Context, h http.Header, id string, actions ...string) (kbacl.Permissions, error) {
	a.calls = append(a.calls, id+":"+strings.Join(actions, ","))
	if h.Get("X-API-Key") == "internal" {
		return kbacl.Permissions{}, fail(500, "test.elevated_identity_leaked")
	}
	p := a.permissions[id]
	if !p.Visible {
		return p, fail(404, "kb_acl.not_found")
	}
	for _, action := range actions {
		if action == "manage" && !p.Manage {
			return p, fail(403, "kb_acl.denied")
		}
		if action != "manage" && !p.Manage && !contains(p.Operations, action) {
			return p, fail(403, "kb_acl.denied")
		}
	}
	return p, nil
}
func (a *fakeACL) AllowedKnowledgeBases(ctx context.Context, h http.Header, ids []string) ([]string, error) {
	out := []string{}
	for _, id := range ids {
		if a.permissions[id].Visible {
			out = append(out, id)
		}
	}
	return out, nil
}

type fakeNative struct {
	docs, chunks map[string]string
	responses    map[string]any
	calls        []string
	resolve      func(string) string
}

func (n *fakeNative) KnowledgeBaseForKnowledge(_ context.Context, id string, _ http.Header) (string, error) {
	if n.resolve != nil {
		return n.resolve(id), nil
	}
	if kb := n.docs[id]; kb != "" {
		return kb, nil
	}
	return "", fail(404, "kb_acl.not_found")
}
func (n *fakeNative) KnowledgeBaseForChunk(_ context.Context, id string, _ http.Header) (string, error) {
	if kb := n.chunks[id]; kb != "" {
		return kb, nil
	}
	return "", fail(404, "kb_acl.not_found")
}
func (n *fakeNative) NativeRequest(_ context.Context, method, path string, q url.Values, _ http.Header, _ any) (json.RawMessage, error) {
	n.calls = append(n.calls, method+" "+path)
	if value, ok := n.responses[path]; ok {
		raw, _ := json.Marshal(value)
		return raw, nil
	}
	if strings.HasPrefix(path, "/api/v1/chunks/by-id/") {
		id := strings.TrimPrefix(path, "/api/v1/chunks/by-id/")
		if kb := n.chunks[id]; kb != "" {
			raw, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"id": id, "knowledge_id": "doc", "knowledge_base_id": kb}})
			return raw, nil
		}
	}
	return nil, fail(404, "kb_acl.not_found")
}
func fixture() (*Guard, *fakeACL, *fakeNative) {
	a := &fakeACL{permissions: map[string]kbacl.Permissions{"kb": {Visible: true, Operations: []string{"document.update"}}, "private": {}}}
	n := &fakeNative{docs: map[string]string{"doc": "kb", "secret": "private", "other": "kb"}, chunks: map[string]string{"chunk": "kb", "hidden": "private"}, responses: map[string]any{}}
	g := &Guard{ACL: a, Upstream: n, Audit: func(context.Context, http.Header, AuditEvent) error { return nil }, WithLock: func(_ context.Context, _ http.Header, _ []string, fn func() error) error { return fn() }, WithCollaborator: func(_ context.Context, _ http.Header, _ string, _ []string, fn func(http.Header) error) error {
		h := http.Header{}
		h.Set("X-API-Key", "internal")
		return fn(h)
	}}
	return g, a, n
}
func run(g *Guard, method, path, body string, next http.HandlerFunc) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer human")
	r.Header.Set("Idempotency-Key", "test-creation")
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	if next == nil {
		next = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }
	}
	g.Wrap(next).ServeHTTP(w, r)
	return w
}
func TestHiddenKnowledgeAndChunkNeverReachNative(t *testing.T) {
	for _, path := range []string{"/api/v1/knowledge/secret", "/api/v1/chunks/by-id/hidden"} {
		t.Run(path, func(t *testing.T) {
			g, _, _ := fixture()
			w := run(g, "GET", path, "", func(w http.ResponseWriter, r *http.Request) { t.Fatal("unauthorized request forwarded") })
			if w.Code != 404 {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
		})
	}
}
func TestChunkDocumentParentIsVerified(t *testing.T) {
	g, _, _ := fixture()
	w := run(g, "GET", "/api/v1/chunks/other/chunk/revisions", "", nil)
	if w.Code != 403 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestBatchEveryIDChecked(t *testing.T) {
	for _, body := range []string{`{"kb_id":"kb","ids":["doc","secret"]}`, `{"kb_id":"private","ids":["doc"]}`} {
		g, a, _ := fixture()
		a.permissions["kb"] = kbacl.Permissions{Visible: true, Manage: true}
		w := run(g, "POST", "/api/v1/knowledge/batch-delete", body, nil)
		if w.Code != 403 {
			t.Fatalf("status %d: %s", w.Code, w.Body)
		}
	}
}
func TestBatchTagsUpdatesKeysCannotHideTarget(t *testing.T) {
	g, _, _ := fixture()
	w := run(g, "PUT", "/api/v1/knowledge/tags", `{"kb_id":"kb","updates":{"secret":[]}}`, nil)
	if w.Code != 403 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}
func TestReparseNeedsAllThreeOperations(t *testing.T) {
	g, _, _ := fixture()
	w := run(g, "POST", "/api/v1/knowledge/doc/reparse", `{}`, nil)
	if w.Code != 403 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestCollaboratorFixedWritePreservesOriginalIdentity(t *testing.T) {
	g, _, _ := fixture()
	called := false
	w := run(g, "PUT", "/api/v1/knowledge/doc", `{"title":"updated"}`, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("X-API-Key") != "internal" || r.Header.Get("Authorization") != "" {
			t.Fatal("missing service credential")
		}
		if OriginalHeaders(r).Get("Authorization") != "Bearer human" {
			t.Fatal("lost actual actor")
		}
		w.WriteHeader(204)
	})
	if w.Code != 204 || !called {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}
func TestCollaboratorUnknownFieldAndParseConfigDenied(t *testing.T) {
	for _, input := range []struct{ path, body string }{{"/api/v1/knowledge/doc", `{"title":"x","creator_id":"attacker"}`}, {"/api/v1/knowledge/doc", `{"process_config":{"chunk_size":4}}`}} {
		g, _, _ := fixture()
		w := run(g, "PUT", input.path, input.body, nil)
		if w.Code != 400 {
			t.Fatalf("status %d", w.Code)
		}
	}
}
func TestMachineNeverUsesDelegation(t *testing.T) {
	g, _, _ := fixture()
	g.WithCollaborator = func(context.Context, http.Header, string, []string, func(http.Header) error) error {
		t.Fatal("public machine elevated")
		return nil
	}
	r := httptest.NewRequest("PUT", "/api/v1/knowledge/doc", strings.NewReader(`{"title":"x"}`))
	r.Header.Set("X-API-Key", "public")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "public" {
			t.Fatal("key changed")
		}
		w.WriteHeader(204)
	})).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestRoleRevocationRecheckedInsideLock(t *testing.T) {
	g, a, _ := fixture()
	g.WithLock = func(_ context.Context, _ http.Header, _ []string, fn func() error) error {
		a.permissions["kb"] = kbacl.Permissions{}
		return fn()
	}
	w := run(g, "PUT", "/api/v1/knowledge/doc", `{"title":"x"}`, func(w http.ResponseWriter, r *http.Request) { t.Fatal("revoked write accepted") })
	if w.Code != 404 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestConcurrentParentMoveFailsClosed(t *testing.T) {
	g, _, n := fixture()
	g.WithLock = func(_ context.Context, _ http.Header, _ []string, fn func() error) error {
		n.docs["doc"] = "private"
		return fn()
	}
	w := run(g, "PUT", "/api/v1/knowledge/doc", `{"title":"x"}`, nil)
	if w.Code != 409 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestPreviewRequiresDownload(t *testing.T) {
	for _, suffix := range []string{"preview", "download"} {
		g, _, _ := fixture()
		w := run(g, "GET", "/api/v1/knowledge/doc/"+suffix, "", nil)
		if w.Code != 403 {
			t.Fatalf("%s status %d", suffix, w.Code)
		}
	}
}
func TestUnsafeEndpointsExplicitlyUnavailable(t *testing.T) {
	for _, path := range []string{"/api/v1/knowledge-bases/copy", "/api/v1/knowledge/move", "/api/v1/knowledgebase/kb/wiki/auto-fix", "/api/v1/faq/import/progress/task", "/api/v1/knowledge-bases/kb/future"} {
		g, _, _ := fixture()
		w := run(g, "POST", path, `{}`, nil)
		if w.Code != 409 {
			t.Fatalf("%s status %d", path, w.Code)
		}
	}
}
func TestResourceClaimInEditableContentDoesNotAuthorizeFile(t *testing.T) {
	g, _, _ := fixture()
	w := run(g, "GET", "/api/v1/knowledge-bases/kb/files?file_path=resource%3A%2F%2Fsecret", "", nil)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "resource_binding") {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}
func TestCreatorForgeryDenied(t *testing.T) {
	g, _, _ := fixture()
	w := run(g, "POST", "/api/v1/knowledge-bases", `{"name":"x","creator_id":"other"}`, nil)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestNativeContentInventoryIsCovered(t *testing.T) {
	raw, err := os.ReadFile("../../../../config/r3-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Routes []struct{ Method, Path, Disposition string }
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range manifest.Routes {
		if r.Disposition != "native" {
			continue
		}
		relevant := false
		for _, prefix := range []string{"/api/v1/knowledge", "/api/v1/chunks", "/api/v1/faq/", "/api/v1/shared-knowledge-bases", "/api/v1/initialization/config/", "/api/v1/initialization/initialize/"} {
			if strings.HasPrefix(r.Path, prefix) {
				relevant = true
			}
		}
		if relevant {
			count++
			if !Classified(r.Method, r.Path) {
				t.Errorf("uncovered %s %s", r.Method, r.Path)
			}
			rt, _, _ := classify(r.Method, r.Path)
			if rt.kind != "disabled" && routeShape(rt.pattern) != routeShape(strings.TrimPrefix(r.Path, "/api/v1")) {
				t.Errorf("literal route accidentally admitted as resource ID: %s %s via %s", r.Method, r.Path, rt.pattern)
			}
		}
	}
	if count < 80 {
		t.Fatalf("unexpected inventory size %d", count)
	}
}
func routeShape(path string) string {
	parts := strings.Split(path, "/")
	for i, v := range parts {
		if strings.HasPrefix(v, ":") {
			parts[i] = ":"
		}
		if strings.HasPrefix(v, "*") {
			parts[i] = "*"
		}
	}
	return strings.Join(parts, "/")
}

func filtered(t *testing.T, g *Guard, method, path, requestBody, responseBody string) (string, error) {
	t.Helper()
	var result string
	var filteredErr error
	w := run(g, method, path, requestBody, func(w http.ResponseWriter, r *http.Request) {
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody)), Request: r}
		filteredErr = g.FilterResponse(resp)
		if filteredErr == nil {
			raw, _ := io.ReadAll(resp.Body)
			result = string(raw)
		}
		w.WriteHeader(200)
	})
	if w.Code != 200 {
		t.Fatalf("request status %d: %s", w.Code, w.Body)
	}
	return result, filteredErr
}
func TestListFilteringRecomputesTotalsAndStripsPaths(t *testing.T) {
	g, _, _ := fixture()
	body, err := filtered(t, g, "GET", "/api/v1/knowledge-bases", "", `{"success":true,"total":2,"data":[{"id":"kb","name":"visible","file_path":"resource://original"},{"id":"private","name":"secret"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "file_path") || !strings.Contains(body, `"total":1`) {
		t.Fatal(body)
	}
}
func TestCollaboratorResponseUsesHumanIdentity(t *testing.T) {
	g, _, _ := fixture()
	body, err := filtered(t, g, "PUT", "/api/v1/knowledge/doc", `{"title":"x"}`, `{"success":true,"data":{"id":"doc","knowledge_base_id":"kb","file_path":"resource://original","source":"https://source"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "file_path") || strings.Contains(body, "source") {
		t.Fatal(body)
	}
}
func TestCreatedKnowledgeBaseMustBindBeforeResponse(t *testing.T) {
	g, a, _ := fixture()
	bound := false
	g.BeforeCreate = func(context.Context, http.Header, string, string) error { return nil }
	g.AfterCreate = func(_ context.Context, h http.Header, key, id string) error {
		if id != "created" || h.Get("Authorization") != "Bearer human" {
			t.Fatal("bad creator binding")
		}
		bound = true
		a.permissions[id] = kbacl.Permissions{Visible: true, Manage: true}
		return nil
	}
	_, err := filtered(t, g, "POST", "/api/v1/knowledge-bases", `{"name":"new"}`, `{"success":true,"data":{"id":"created"}}`)
	if err != nil || !bound {
		t.Fatalf("bound %v err %v", bound, err)
	}
}
func TestDeleteTombstoneOnNativeSuccess(t *testing.T) {
	g, a, _ := fixture()
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Manage: true}
	deleted := false
	g.MarkDeleted = func(context.Context, http.Header, string) error { deleted = true; return nil }
	_, err := filtered(t, g, "DELETE", "/api/v1/knowledge-bases/kb", "", `{"success":true}`)
	if err != nil || !deleted {
		t.Fatalf("deleted %v err %v", deleted, err)
	}
}
func TestExportRequiresGrantAndActualParent(t *testing.T) {
	g, a, n := fixture()
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"chunk.export"}}
	n.responses["/api/v1/chunks/doc"] = map[string]any{"success": true, "total": 1, "data": []any{map[string]any{"id": "chunk", "knowledge_id": "doc", "knowledge_base_id": "kb", "content": "approved", "file_path": "secret"}}}
	w := run(g, "POST", "/api/v1/mindcreek/kb-permissions/kb/export-chunks", `{"knowledge_ids":["doc"]}`, func(http.ResponseWriter, *http.Request) { t.Fatal("export should be local") })
	if w.Code != 200 || !strings.Contains(w.Body.String(), "approved") || strings.Contains(w.Body.String(), "file_path") || w.Header().Get("Content-Disposition") == "" {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}

func TestFAQSingleCreateAndMutationPermissions(t *testing.T) {
	g, a, n := fixture()
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"document.create"}}
	w := run(g, "POST", "/api/v1/knowledge-bases/kb/faq/entry", `{"standard_question":"question","answers":["answer"],"tag_id":0,"tag_name":""}`, nil)
	if w.Code != 204 {
		t.Fatalf("create status %d: %s", w.Code, w.Body)
	}
	n.responses["/api/v1/knowledge-bases/kb/faq/entries/7"] = map[string]any{"success": true, "data": map[string]any{"knowledge_base_id": "kb", "id": 7}}
	w = run(g, "PUT", "/api/v1/knowledge-bases/kb/faq/entries/7", `{"standard_question":"updated","answers":["answer"]}`, nil)
	if w.Code != 403 {
		t.Fatalf("create grant permitted edit: %d", w.Code)
	}
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"document.update", "chunk.update"}}
	w = run(g, "PUT", "/api/v1/knowledge-bases/kb/faq/entries/7", `{"standard_question":"updated","answers":["answer"]}`, nil)
	if w.Code != 204 {
		t.Fatalf("edit status %d: %s", w.Code, w.Body)
	}
}
func TestFAQBatchCannotChangeForeignEntryOrInventTag(t *testing.T) {
	g, a, n := fixture()
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"document.update", "chunk.update", "document.delete", "document.create"}}
	n.responses["/api/v1/knowledge-bases/kb/faq/entries/8"] = map[string]any{"success": true, "data": map[string]any{"knowledge_base_id": "private", "id": 8}}
	w := run(g, "PUT", "/api/v1/knowledge-bases/kb/faq/entries/fields", `{"by_id":{"8":{"is_enabled":true}}}`, nil)
	if w.Code != 403 {
		t.Fatalf("foreign entry status %d: %s", w.Code, w.Body)
	}
	w = run(g, "POST", "/api/v1/knowledge-bases/kb/faq/entry", `{"standard_question":"q","answers":["a"],"tag_name":"new structure"}`, nil)
	if w.Code != 403 {
		t.Fatalf("new tag status %d: %s", w.Code, w.Body)
	}
}
func TestFAQReplaceRequiresDeleteAndRejectsTaskID(t *testing.T) {
	g, a, _ := fixture()
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"document.create", "document.update", "chunk.update"}}
	w := run(g, "POST", "/api/v1/knowledge-bases/kb/faq/entries", `{"mode":"replace","entries":[{"standard_question":"q","answers":["a"]}]}`, nil)
	if w.Code != 403 {
		t.Fatalf("replace status %d", w.Code)
	}
	w = run(g, "POST", "/api/v1/knowledge-bases/kb/faq/entries", `{"mode":"append","task_id":"foreign-task","entries":[{"standard_question":"q","answers":["a"]}]}`, nil)
	if w.Code != 400 {
		t.Fatalf("task spoof status %d", w.Code)
	}
}
func TestWikiCRUDAndSourceReferences(t *testing.T) {
	g, a, _ := fixture()
	a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"document.create", "document.update"}}
	w := run(g, "POST", "/api/v1/knowledgebase/kb/wiki/pages", `{"slug":"new","title":"New","content":"text","source_refs":["doc|Title"],"chunk_refs":["chunk"]}`, nil)
	if w.Code != 204 {
		t.Fatalf("wiki create status %d: %s", w.Code, w.Body)
	}
	w = run(g, "POST", "/api/v1/knowledgebase/kb/wiki/pages", `{"slug":"new","source_refs":["secret|hidden"]}`, nil)
	if w.Code != 403 {
		t.Fatalf("wiki foreign source status %d: %s", w.Code, w.Body)
	}
	w = run(g, "PUT", "/api/v1/knowledgebase/kb/wiki/pages/path/nested", `{"content":"new text","version":3}`, nil)
	if w.Code != 204 {
		t.Fatalf("wiki slug edit status %d: %s", w.Code, w.Body)
	}
	w = run(g, "DELETE", "/api/v1/knowledgebase/kb/wiki/pages/path/nested", "", nil)
	if w.Code != 403 {
		t.Fatalf("wiki delete status %d", w.Code)
	}
}
func TestWikiRawResponseSanitizedAndParentChecked(t *testing.T) {
	g, _, _ := fixture()
	body, err := filtered(t, g, "GET", "/api/v1/knowledgebase/kb/wiki/pages/page", "", `{"knowledge_base_id":"kb","title":"readable","file_path":"resource://original"}`)
	if err != nil || strings.Contains(body, "file_path") || !strings.Contains(body, "readable") {
		t.Fatalf("%s err=%v", body, err)
	}
	_, err = filtered(t, g, "GET", "/api/v1/knowledgebase/kb/wiki/pages/page", "", `{"knowledge_base_id":"private","content":"secret"}`)
	if err == nil {
		t.Fatal("foreign wiki response exposed")
	}
}
func TestWikiGraphSourceSlugIsNotAFileLocator(t *testing.T) {
	g, _, _ := fixture()
	body, err := filtered(t, g, "GET", "/api/v1/knowledgebase/kb/wiki/graph", "", `{"nodes":[{"slug":"a"}],"edges":[{"source":"a","target":"b"}]}`)
	if err != nil || !strings.Contains(body, `"source":"a"`) {
		t.Fatalf("%s err=%v", body, err)
	}
}
func TestMetadataSearchQueriesOnlyVisibleKBAndPaginatesFilteredUnion(t *testing.T) {
	g, _, n := fixture()
	n.responses["/api/v1/knowledge-bases"] = map[string]any{"success": true, "data": []any{map[string]any{"id": "kb"}, map[string]any{"id": "private"}}}
	n.responses["/api/v1/knowledge-bases/kb/knowledge"] = map[string]any{"success": true, "total": 2, "data": []any{map[string]any{"id": "a", "knowledge_base_id": "kb", "created_at": "2026-01-01"}, map[string]any{"id": "b", "knowledge_base_id": "kb", "created_at": "2026-01-02"}}}
	w := run(g, "GET", "/api/v1/knowledge/search?keyword=visible&offset=1&limit=1", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":2`) || !strings.Contains(w.Body.String(), `"id":"a"`) || strings.Contains(w.Body.String(), `"id":"b"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, call := range n.calls {
		if strings.Contains(call, "private/knowledge") {
			t.Fatal("queried hidden content before filtering")
		}
	}
}
func TestMultipartCreateAllowsFileOnlyAndRejectsSmuggledIdentity(t *testing.T) {
	for _, forged := range []bool{false, true} {
		g, a, _ := fixture()
		a.permissions["kb"] = kbacl.Permissions{Visible: true, Operations: []string{"document.create"}}
		var buf bytes.Buffer
		form := multipart.NewWriter(&buf)
		part, _ := form.CreateFormFile("file", "example.txt")
		_, _ = part.Write([]byte("content"))
		_ = form.WriteField("fileName", "example.txt")
		if forged {
			_ = form.WriteField("creator_id", "other")
		}
		_ = form.Close()
		r := httptest.NewRequest("POST", "/api/v1/knowledge-bases/kb/knowledge/file", bytes.NewReader(buf.Bytes()))
		r.Header.Set("Authorization", "Bearer human")
		r.Header.Set("Content-Type", form.FormDataContentType())
		w := httptest.NewRecorder()
		g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if forged {
				t.Fatal("forged multipart forwarded")
			}
			raw, _ := io.ReadAll(r.Body)
			if !bytes.Equal(raw, buf.Bytes()) {
				t.Fatal("multipart body changed")
			}
			w.WriteHeader(204)
		})).ServeHTTP(w, r)
		expected := 204
		if forged {
			expected = 400
		}
		if w.Code != expected {
			t.Fatalf("forged %v status %d: %s", forged, w.Code, w.Body)
		}
	}
}
func TestNativeRejectedWriteReportsFailureOnce(t *testing.T) {
	g, _, _ := fixture()
	sawFailure := false
	g.WithCollaborator = func(_ context.Context, _ http.Header, _ string, _ []string, fn func(http.Header) error) error {
		err := fn(http.Header{})
		sawFailure = err != nil
		return err
	}
	w := run(g, "PUT", "/api/v1/knowledge/doc", `{"title":"x"}`, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"native conflict"}`))
	})
	if !sawFailure || w.Body.String() != `{"error":"native conflict"}` {
		t.Fatalf("failure %v body %s", sawFailure, w.Body)
	}
}
func TestCreatePendingPreventsForwarding(t *testing.T) {
	g, _, _ := fixture()
	g.BeforeCreate = func(context.Context, http.Header, string, string) error { return fail(409, "kb_acl.create_pending") }
	g.AfterCreate = func(context.Context, http.Header, string, string) error {
		t.Fatal("completed pending creation")
		return nil
	}
	w := run(g, "POST", "/api/v1/knowledge-bases", `{"name":"new"}`, func(http.ResponseWriter, *http.Request) { t.Fatal("duplicate native creation") })
	if w.Code != 409 {
		t.Fatalf("status %d", w.Code)
	}
}

func TestAuditUsesActualActorAndIdentifiersAndFailureOutcome(t *testing.T) {
	g, _, _ := fixture()
	events := []AuditEvent{}
	g.Audit = func(_ context.Context, h http.Header, e AuditEvent) error {
		if h.Get("Authorization") != "Bearer human" || h.Get("X-API-Key") != "" {
			t.Fatal("audit did not preserve actual actor")
		}
		events = append(events, e)
		return nil
	}
	w := run(g, "PUT", "/api/v1/knowledge/doc", `{"title":"private document contents"}`, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(409) })
	if w.Code != 409 || len(events) != 2 || events[0].Outcome != "attempt" || events[1].Outcome != "failed" || events[1].Status != 409 {
		t.Fatalf("status %d events %#v", w.Code, events)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "private document contents") || !strings.Contains(string(raw), "knowledge:doc") {
		t.Fatal(string(raw))
	}
}
func TestAuditFailurePreventsContentWrite(t *testing.T) {
	g, _, _ := fixture()
	g.Audit = func(context.Context, http.Header, AuditEvent) error { return fail(503, "audit.unavailable") }
	w := run(g, "PUT", "/api/v1/knowledge/doc", `{"title":"x"}`, func(http.ResponseWriter, *http.Request) { t.Fatal("write without durable audit") })
	if w.Code != 503 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestFAQProgressResolvesAuthoritativeParentBeforeReturningSnapshot(t *testing.T) {
	g, _, n := fixture()
	n.responses["/api/v1/faq/import/progress/task"] = map[string]any{"success": true, "data": map[string]any{"task_id": "task", "kb_id": "kb", "knowledge_id": "doc", "failed_entries_url": "resource://original", "progress": 50}}
	w := run(g, "GET", "/api/v1/faq/import/progress/task", "", func(http.ResponseWriter, *http.Request) { t.Fatal("progress should return checked snapshot") })
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"progress":50`) || strings.Contains(w.Body.String(), "failed_entries_url") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(n.calls) != 1 || n.calls[0] != "GET /api/v1/faq/import/progress/task" {
		t.Fatalf("progress must use one authoritative snapshot: %v", n.calls)
	}
	n.responses["/api/v1/faq/import/progress/task"] = map[string]any{"success": true, "data": map[string]any{"task_id": "task", "kb_id": "private", "knowledge_id": "secret", "progress": 70}}
	w = run(g, "GET", "/api/v1/faq/import/progress/task", "", nil)
	if w.Code != 404 || strings.Contains(w.Body.String(), "progress") {
		t.Fatalf("private task %d %s", w.Code, w.Body)
	}
	for _, tc := range []struct {
		name string
		data map[string]any
		want int
	}{
		{"missing knowledge", map[string]any{"task_id": "task", "kb_id": "kb"}, 409},
		{"missing KB", map[string]any{"task_id": "task", "knowledge_id": "doc"}, 409},
		{"different task", map[string]any{"task_id": "other", "kb_id": "kb", "knowledge_id": "doc"}, 409},
		{"forged parent", map[string]any{"task_id": "task", "kb_id": "kb", "knowledge_id": "secret"}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, n := fixture()
			tc.data["progress"] = 60
			n.responses["/api/v1/faq/import/progress/task"] = map[string]any{"success": true, "data": tc.data}
			w := run(g, "GET", "/api/v1/faq/import/progress/task", "", nil)
			if w.Code != tc.want || strings.Contains(w.Body.String(), "progress") || len(n.calls) != 1 {
				t.Fatalf("status=%d calls=%v body=%s", w.Code, n.calls, w.Body)
			}
		})
	}
}
