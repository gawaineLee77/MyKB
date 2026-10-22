package nativeaccess

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
)

func TestProductACLFiltersBeforeRetrievalAndRechecksHistory(t *testing.T) {
	s, f, store, actor := testScope()
	f.allowed["private"] = true // Native same-tenant access would permit this KB.
	visible := map[string]bool{"a": true}
	s.KnowledgeAccess = func(_ context.Context, id string, _ http.Header) error {
		if !visible[id] {
			return &kbacl.Error{Status: 404, Code: "kb_acl.not_found"}
		}
		return nil
	}
	ids, err := s.Defaults(context.Background(), headerForACL())
	if err != nil || len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("defaults %v %v", ids, err)
	}
	for _, call := range f.requests {
		if strings.Contains(call, "shared-knowledge") {
			t.Fatal("enumerated unsupported cross-workspace sharing")
		}
	}
	for _, body := range []string{`{"knowledge_base_ids":["private"]}`, `{"knowledge_ids":["private-file"]}`, `{"mentioned_items":[{"type":"file","id":"private-file","kb_id":"a"}]}`, `{"tag_scopes":[{"knowledge_base_id":"private","tag_ids":["tag"]}]}`} {
		r := requestJSON("POST", "/api/v1/knowledge-search", body)
		if err := s.Check(r.Context(), r, actor); err == nil {
			t.Fatalf("private target reached retrieval: %s", body)
		}
	}
	r := requestJSON("POST", "/api/v1/knowledge-search", `{"query":"synthetic"}`)
	if err := s.Check(r.Context(), r, actor); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if ids := stringsValue(body["knowledge_base_ids"]); len(ids) != 1 || ids[0] != "a" {
		t.Fatal(body)
	}
	if err := store.Bind(context.Background(), "session", actor, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	visible["a"] = false
	if _, err := s.CheckSession(context.Background(), "session", actor, headerForACL()); err == nil {
		t.Fatal("revoked history remained readable")
	}
	if _, err := s.Resolve(context.Background(), nil, false, "", actor, headerForACL()); err == nil {
		t.Fatal("empty scope became unbounded")
	}
}
func headerForACL() http.Header { return http.Header{"Authorization": []string{"Bearer synthetic"}} }

func TestProductACLAgentAllAndWriteTools(t *testing.T) {
	s, f, _, actor := testScope()
	s.KnowledgeAccess = func(_ context.Context, id string, _ http.Header) error {
		if id != "a" {
			return &kbacl.Error{Status: 404, Code: "kb_acl.not_found"}
		}
		return nil
	}
	f.caps = map[string]map[string]any{"a": {"vector": true}}
	f.agent["config"] = map[string]any{"agent_mode": "smart-reasoning", "kb_selection_mode": "all", "allowed_tools": []any{"knowledge_search"}, "mcp_selection_mode": "none", "memory_enabled": false}
	ids, err := s.Resolve(context.Background(), nil, false, "agent", actor, headerForACL())
	if err != nil || len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("agent all %v %v", ids, err)
	}
	f.agent["config"].(map[string]any)["allowed_tools"] = []any{"wiki_write_page"}
	s.KnowledgeManage = func(context.Context, string, http.Header) error {
		return &kbacl.Error{Status: 403, Code: "kb_acl.operation_denied"}
	}
	if _, err := s.Resolve(context.Background(), []string{"a"}, true, "agent", actor, headerForACL()); err == nil {
		t.Fatal("collaborator acquired internal Agent writes")
	}
	f.agent["config"].(map[string]any)["allowed_tools"] = []any{"new_unreviewed_tool"}
	if _, err := s.Resolve(context.Background(), []string{"a"}, true, "agent", actor, headerForACL()); err == nil {
		t.Fatal("unclassified tool accepted")
	}
	f.agent["tenant_id"] = float64(2)
	if _, err := s.Resolve(context.Background(), nil, false, "agent", actor, headerForACL()); err == nil {
		t.Fatal("cross-workspace Agent accepted")
	}
}

func TestAgentMetadataDoesNotExposeInvisibleKBs(t *testing.T) {
	s, _, _, _ := testScope()
	s.KnowledgeAccess = func(_ context.Context, id string, _ http.Header) error {
		if id == "private" {
			return &kbacl.Error{Status: 404, Code: "kb_acl.not_found"}
		}
		return nil
	}
	r := requestJSON("GET", "/api/v1/agents", "")
	response := &http.Response{StatusCode: 200, Request: r, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":[{"id":"hidden","config":{"knowledge_bases":["private"]}},{"id":"visible","config":{"knowledge_bases":["a"]}}],"disabled_own_agent_ids":["hidden","visible"],"total":2}`))}
	if err := s.FilterAgentVisibility(response); err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(response.Body)
	if strings.Contains(string(out), "hidden") || strings.Contains(string(out), "private") || !strings.Contains(string(out), `"total":1`) {
		t.Fatal(string(out))
	}
}
