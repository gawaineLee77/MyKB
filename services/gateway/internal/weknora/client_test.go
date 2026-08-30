package weknora

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestManualKnowledgeContracts(t *testing.T) {
	manualJSON := `{"id":"note-1","knowledge_base_id":"kb-notes","type":"manual","title":"Daily log","parse_status":"completed","metadata":{"content":"# Daily log\nHello","format":"markdown","status":"publish","version":2,"updated_at":"2026-08-26T08:00:00Z"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer contract-token" {
			http.Error(w, "missing credential", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/knowledge-bases/kb-notes/knowledge":
			if r.URL.Query().Get("source") != "manual" || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("page_size") != "10" {
				http.Error(w, "invalid list query", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[` + manualJSON + `],"total":1,"page":1,"page_size":10}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/knowledge/note-1":
			_, _ = w.Write([]byte(`{"success":true,"data":` + manualJSON + `}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/knowledge-bases/kb-notes/knowledge/manual":
			assertManualInput(t, r, "New note", "publish")
			_, _ = w.Write([]byte(`{"success":true,"data":` + manualJSON + `}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/knowledge/manual/note-1":
			assertManualInput(t, r, "Updated note", "draft")
			_, _ = w.Write([]byte(`{"success":true,"data":` + manualJSON + `}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/knowledge/note-1":
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	client := newTestClient(t, upstream.URL, 2*time.Second)
	headers := http.Header{"Authorization": {"Bearer contract-token"}}
	page, err := client.ListManualKnowledge(context.Background(), "kb-notes", 1, 10, headers)
	if err != nil || page.Total != 1 || page.Items[0].Metadata.Version != 2 {
		t.Fatalf("ListManualKnowledge() = %+v, %v", page, err)
	}
	note, err := client.GetManualKnowledge(context.Background(), "kb-notes", "note-1", headers)
	if err != nil || note.Metadata.Content != "# Daily log\nHello" {
		t.Fatalf("GetManualKnowledge() = %+v, %v", note, err)
	}
	created, err := client.CreateManualKnowledge(context.Background(), "kb-notes", ManualKnowledgeInput{Title: "New note", Content: "body", Status: "publish"}, headers)
	if err != nil || created.ID != "note-1" {
		t.Fatalf("CreateManualKnowledge() = %+v, %v", created, err)
	}
	updated, err := client.UpdateManualKnowledge(context.Background(), "kb-notes", "note-1", ManualKnowledgeInput{Title: "Updated note", Content: "body", Status: "draft"}, headers)
	if err != nil || updated.ID != "note-1" {
		t.Fatalf("UpdateManualKnowledge() = %+v, %v", updated, err)
	}
	if err := client.DeleteManualKnowledge(context.Background(), "note-1", headers); err != nil {
		t.Fatalf("DeleteManualKnowledge() error = %v", err)
	}
}

func TestDocumentIngestionContracts(t *testing.T) {
	documentJSON := `{"id":"doc-1","knowledge_base_id":"kb-rag","type":"file","title":"guide.md","file_name":"guide.md","file_type":"md","file_size":18,"parse_status":"pending"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/knowledge-bases/kb-rag/knowledge/file":
			file, header, err := r.FormFile("file")
			if err != nil || header.Filename != "guide.md" || r.FormValue("channel") != "mindcreek" {
				http.Error(w, "invalid upload", http.StatusBadRequest)
				return
			}
			defer file.Close()
			var content struct{ Value string }
			raw := make([]byte, 18)
			n, _ := file.Read(raw)
			content.Value = string(raw[:n])
			if content.Value != "# Synthetic guide" {
				http.Error(w, "invalid content", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":` + documentJSON + `}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/knowledge-bases/kb-rag/knowledge":
			_, _ = w.Write([]byte(`{"success":true,"data":[` + documentJSON + `],"total":1,"page":1,"page_size":20}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/knowledge/doc-1":
			_, _ = w.Write([]byte(`{"success":true,"data":` + documentJSON + `}`))
		case r.Method == http.MethodPost && (r.URL.Path == "/api/v1/knowledge/doc-1/reparse" || r.URL.Path == "/api/v1/knowledge/doc-1/cancel-parse"):
			_, _ = w.Write([]byte(`{"success":true,"data":` + documentJSON + `}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := newTestClient(t, upstream.URL, 2*time.Second)
	created, err := client.UploadKnowledge(context.Background(), "kb-rag", "guide.md", strings.NewReader("# Synthetic guide"), nil)
	if err != nil || created.ID != "doc-1" {
		t.Fatalf("UploadKnowledge() = %+v, %v", created, err)
	}
	page, err := client.ListKnowledge(context.Background(), "kb-rag", 1, 20, nil)
	if err != nil || page.Total != 1 {
		t.Fatalf("ListKnowledge() = %+v, %v", page, err)
	}
	if _, err := client.GetKnowledge(context.Background(), "kb-rag", "doc-1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReparseKnowledge(context.Background(), "kb-rag", "doc-1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CancelKnowledge(context.Background(), "kb-rag", "doc-1", nil); err != nil {
		t.Fatal(err)
	}
}

func assertManualInput(t *testing.T, r *http.Request, title, status string) {
	t.Helper()
	var input ManualKnowledgeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		t.Fatal(err)
	}
	if input.Title != title || input.Status != status || input.Content != "body" {
		t.Fatalf("manual input = %+v", input)
	}
}

func TestV072Contracts(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/system/info":
			if r.Header.Get("Authorization") != "Bearer contract-token" {
				http.Error(w, "missing credential", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"version":"v0.7.2","edition":"standard","commit_id":"3d5d8bf"}}`))
		case "/api/v1/auth/me":
			if r.Header.Get("X-Tenant-ID") != "42" {
				http.Error(w, "missing tenant", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"user":{"id":"user-1","email":"user@example.test","tenant_id":42},"tenant":{"id":42,"name":"Test"},"memberships":[{"tenant_id":42,"role":"owner"}]}}`))
		case "/api/v1/knowledge/knowledge-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"knowledge-1","knowledge_base_id":"kb-1"}}`))
		case "/api/v1/chunks/by-id/chunk-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"chunk-1","knowledge_base_id":"kb-1"}}`))
		case "/api/v1/sessions/session-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"session-1","tenant_id":42}}`))
		case "/api/v1/agents/agent-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"agent-1","tenant_id":42,"config":{"kb_selection_mode":"selected","knowledge_bases":["kb-1"]}}}`))
		case "/api/v1/knowledge-bases/kb-deterministic":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"kb-deterministic","name":"Notes","type":"document","tenant_id":42,"creator_id":"user-1","embedding_model_id":"model-1"}}`))
		case "/api/v1/knowledge-bases":
			if r.Header.Get("Authorization") != "Bearer contract-token" {
				http.Error(w, "invalid create request", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"kb-deterministic","name":"Notes","type":"document","tenant_id":42,"creator_id":"user-1"}]}`))
				return
			}
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"success":true,"data":{"id":"kb-created","name":"Notes","type":"document","tenant_id":42,"creator_id":"user-1","embedding_model_id":"model-1"}}`))
				return
			}
			http.Error(w, "invalid method", http.StatusMethodNotAllowed)
		case "/api/v1/tenants/42/members":
			if r.URL.Query().Get("q") != "bob" || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("page_size") != "20" {
				http.Error(w, "invalid member query", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"members":[{"user_id":"user-2","email":"bob@example.test","username":"Bob","role":"viewer","status":"active"}],"total":1,"page":1,"page_size":20}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	client := newTestClient(t, upstream.URL, 2*time.Second)
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer contract-token")
	headers.Set("X-Tenant-ID", "42")
	info, err := client.Version(context.Background(), headers)
	if err != nil || info.Version != SupportedVersion {
		t.Fatalf("Version() = %+v, %v", info, err)
	}
	principal, err := client.CurrentPrincipal(context.Background(), headers)
	if err != nil || principal.User == nil || principal.User.ID != "user-1" || principal.Tenant == nil || principal.Tenant.ID != 42 {
		t.Fatalf("CurrentPrincipal() = %+v, %v", principal, err)
	}
	knowledgeKB, err := client.KnowledgeBaseForKnowledge(context.Background(), "knowledge-1", headers)
	if err != nil || knowledgeKB != "kb-1" {
		t.Fatalf("KnowledgeBaseForKnowledge() = %q, %v", knowledgeKB, err)
	}
	chunkKB, err := client.KnowledgeBaseForChunk(context.Background(), "chunk-1", headers)
	if err != nil || chunkKB != "kb-1" {
		t.Fatalf("KnowledgeBaseForChunk() = %q, %v", chunkKB, err)
	}
	if err := client.ValidateSession(context.Background(), "session-1", headers); err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	agentScope, err := client.AgentKnowledgeBases(context.Background(), "agent-1", headers)
	if err != nil || len(agentScope.KnowledgeBaseIDs) != 1 || agentScope.KnowledgeBaseIDs[0] != "kb-1" {
		t.Fatalf("AgentKnowledgeBases() = %+v, %v", agentScope, err)
	}
	kb, err := client.GetKnowledgeBase(context.Background(), "kb-deterministic", headers)
	if err != nil || kb.CreatorID != "user-1" {
		t.Fatalf("GetKnowledgeBase() = %+v, %v", kb, err)
	}
	listed, err := client.ListKnowledgeBases(context.Background(), headers)
	if err != nil || len(listed) != 1 || listed[0].ID != "kb-deterministic" {
		t.Fatalf("ListKnowledgeBases() = %+v, %v", listed, err)
	}
	members, err := client.ListTenantMembers(context.Background(), 42, "bob", 1, 20, headers)
	if err != nil || members.Total != 1 || members.Items[0].UserID != "user-2" {
		t.Fatalf("ListTenantMembers() = %+v, %v", members, err)
	}
	created, err := client.CreateKnowledgeBase(context.Background(), CreateKnowledgeBaseRequest{
		ID: "kb-created", Name: "Notes", Type: "document", EmbeddingModelID: "model-1",
	}, headers)
	if err != nil || created.ID != "kb-created" {
		t.Fatalf("CreateKnowledgeBase() = %+v, %v", created, err)
	}
}

func TestUnsupportedConfiguredVersionFailsClosed(t *testing.T) {
	base, _ := url.Parse("http://upstream.example")
	_, err := New(base, "v0.8.0", time.Second)
	assertAdapterError(t, err, "upstream.version_unsupported", http.StatusServiceUnavailable)
}

func TestBuiltinAgentAllScopeDoesNotUseSharedAgentLookup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/builtin-smart-reasoning" {
			t.Fatalf("unexpected shared-agent lookup: %s", r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"builtin-smart-reasoning","tenant_id":42,"config":{"kb_selection_mode":"all","knowledge_bases":[]}}}`))
	}))
	defer upstream.Close()

	scope, err := newTestClient(t, upstream.URL, time.Second).AgentKnowledgeBases(
		context.Background(), "builtin-smart-reasoning", nil,
	)
	if err != nil || scope.SelectionMode != "all" || len(scope.KnowledgeBaseIDs) != 0 {
		t.Fatalf("AgentKnowledgeBases() = %+v, %v", scope, err)
	}
}

func TestLiveVersionMismatchFailsClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"version":"v0.8.0"}}`))
	}))
	defer upstream.Close()
	_, err := newTestClient(t, upstream.URL, time.Second).Version(context.Background(), nil)
	assertAdapterError(t, err, "upstream.version_unsupported", http.StatusServiceUnavailable)
}

func TestStatusAndTimeoutTranslation(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "secret upstream detail", http.StatusUnauthorized)
		}))
		defer upstream.Close()
		_, err := newTestClient(t, upstream.URL, time.Second).CurrentPrincipal(context.Background(), nil)
		assertAdapterError(t, err, "upstream.unauthorized", http.StatusUnauthorized)
	})

	t.Run("timeout", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		defer upstream.Close()
		err := newTestClient(t, upstream.URL, 10*time.Millisecond).Health(context.Background())
		assertAdapterError(t, err, "upstream.timeout", http.StatusBadGateway)
	})
}

func TestPhase4SearchExcerptAndAgentContracts(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer phase4" {
			http.Error(w, "missing credential", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/knowledge-search":
			var input SearchRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.Query != "river" || len(input.KnowledgeBaseIDs) != 1 || input.KnowledgeBaseIDs[0] != "kb-a" {
				http.Error(w, "invalid search scope", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"chunk-1","content":"grounded","knowledge_base_id":"kb-a","knowledge_id":"doc-1","knowledge_title":"Guide"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/chunks/by-id/chunk-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"chunk-1","content":"grounded excerpt","knowledge_base_id":"kb-a","knowledge_id":"doc-1","chunk_index":1,"chunk_type":"text"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"session-1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/knowledge-chat/session-1":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"response_type\":\"references\",\"knowledge_references\":[{\"id\":\"chunk-1\",\"content\":\"grounded\",\"knowledge_base_id\":\"kb-a\",\"knowledge_id\":\"doc-1\"}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"response_type\":\"answer\",\"content\":\"The answer\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"response_type\":\"complete\"}\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := newTestClient(t, upstream.URL, 2*time.Second)
	headers := http.Header{"Authorization": {"Bearer phase4"}}
	results, err := client.SearchKnowledge(context.Background(), SearchRequest{Query: "river", KnowledgeBaseIDs: []string{"kb-a"}}, headers)
	if err != nil || len(results) != 1 || results[0].KnowledgeBaseID != "kb-a" {
		t.Fatalf("SearchKnowledge() = %+v, %v", results, err)
	}
	excerpt, err := client.GetChunkExcerpt(context.Background(), "chunk-1", headers)
	if err != nil || excerpt.Content != "grounded excerpt" {
		t.Fatalf("GetChunkExcerpt() = %+v, %v", excerpt, err)
	}
	session, err := client.CreateChatSession(context.Background(), "MCP", headers)
	if err != nil || session.ID != "session-1" {
		t.Fatalf("CreateChatSession() = %+v, %v", session, err)
	}
	answer, err := client.AskKnowledge(context.Background(), session.ID, "river?", "", []string{"kb-a"}, headers)
	if err != nil || answer.Answer != "The answer" || len(answer.References) != 1 {
		t.Fatalf("AskKnowledge() = %+v, %v", answer, err)
	}
}

func newTestClient(t *testing.T, rawURL string, timeout time.Duration) *Client {
	t.Helper()
	base, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(base, SupportedVersion, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertAdapterError(t *testing.T, err error, code string, status int) {
	t.Helper()
	var adapterError *Error
	if !errors.As(err, &adapterError) {
		t.Fatalf("error = %T %v, want *Error", err, err)
	}
	if adapterError.Code != code || adapterError.StatusCode != status {
		t.Fatalf("error = %+v, want code=%s status=%d", adapterError, code, status)
	}
}
