package nativeaccess

import (
	"context"
	"net/http"
	"testing"
)

func safeAgentConfig() map[string]any {
	return map[string]any{"agent_mode": "smart-reasoning", "kb_selection_mode": "selected", "knowledge_bases": []any{"a"}, "allowed_tools": []any{"knowledge_search"}, "mcp_selection_mode": "none", "mcp_services": []any{}, "skills_selection_mode": "none", "selected_skills": []any{}, "sandbox_config_id": "", "web_search_enabled": false, "memory_enabled": false}
}

func TestAgentImplicitRegistrarsAndUnsafeToolsAreDisabled(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"default MCP expands all", "mcp_selection_mode", ""}, {"all MCP", "mcp_selection_mode", "all"}, {"selected MCP", "mcp_services", []any{"service"}},
		{"all skills", "skills_selection_mode", "all"}, {"selected skills", "selected_skills", []any{"skill"}}, {"sandbox file tools", "sandbox_config_id", "remote"}, {"automatic web tools", "web_search_enabled", true},
		{"wiki write", "allowed_tools", []any{"wiki_write_page"}}, {"SQL metadata", "allowed_tools", []any{"database_query"}}, {"shared DuckDB", "allowed_tools", []any{"data_analysis"}},
		{"explicit memory", "memory_enabled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := safeAgentConfig()
			config[tc.key] = tc.value
			if err := validateAgentIsolation(config); err == nil {
				t.Fatal("unsafe engine admitted")
			}
		})
	}
	config := safeAgentConfig()
	config["allowed_tools"] = []any{"knowledge_search", "get_document_info", "data_schema", "wiki_read_page"}
	if err := validateAgentIsolation(config); err != nil {
		t.Fatal(err)
	}
}

func TestSafeAgentCannotBeMutatedOrCopiedIntoUnsafeConfiguration(t *testing.T) {
	s, f, _, actor := testScope()
	s.KnowledgeAccess = func(context.Context, string, http.Header) error { return nil }
	s.PlainChatAllowed = func(context.Context, http.Header) error { return nil }
	f.agent["config"] = safeAgentConfig()
	r := requestJSON("PUT", "/api/v1/agents/agent", `{"config":{"agent_mode":"quick-answer"}}`)
	if err := s.Check(r.Context(), r, actor); err == nil {
		t.Fatal("saved an Agent without explicit memory opt-out")
	}
	for _, body := range []string{`{"config":{"agent_mode":"smart-reasoning","mcp_selection_mode":"none","allowed_tools":["wiki_write_page"]}}`, `{"config":{"agent_mode":"smart-reasoning","mcp_selection_mode":"all","allowed_tools":["knowledge_search"]}}`, `{"config":{"agent_mode":"smart-reasoning","mcp_selection_mode":"none","allowed_tools":["knowledge_search"],"sandbox_config_id":"backend"}}`} {
		r := requestJSON("PUT", "/api/v1/agents/agent", body)
		if err := s.Check(r.Context(), r, actor); err == nil {
			t.Fatalf("unsafe mutation accepted: %s", body)
		}
	}
	f.agent["config"].(map[string]any)["allowed_tools"] = []any{"wiki_delete_page"}
	r = requestJSON("POST", "/api/v1/agents/agent/copy", `{"name":"copy"}`)
	if err := s.Check(r.Context(), r, actor); err == nil {
		t.Fatal("copied legacy unsafe tools")
	}
	r = requestJSON("POST", "/api/v1/agents/agent/copy", `{"config":{"agent_mode":"quick-answer"}}`)
	if err := s.Check(r.Context(), r, actor); err == nil {
		t.Fatal("copy ignored unsafe source because of unused request config")
	}
}

func TestAgentRequestCannotAddExternalToolsOrReuseSandboxPin(t *testing.T) {
	s, f, store, actor := testScope()
	s.KnowledgeAccess = func(context.Context, string, http.Header) error { return nil }
	s.PlainChatAllowed = func(context.Context, http.Header) error { return nil }
	f.agent["config"] = safeAgentConfig()
	if err := store.Bind(context.Background(), "session", actor, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"agent_id":"agent","mcp_service_ids":["service"]}`, `{"agent_id":"agent","skill_names":["skill"]}`, `{"agent_id":"agent","web_search_enabled":true}`, `{"agent_id":"agent","mentioned_items":[{"type":"mcp","id":"service"}]}`, `{"agent_id":"agent","mentioned_items":[{"type":"skill","id":"script"}]}`} {
		r := requestJSON("POST", "/api/v1/agent-chat/session", body)
		if err := s.Check(r.Context(), r, actor); err == nil {
			t.Fatal("request extended safe agent", body)
		}
	}
	f.sandboxPin = "old-backend"
	if _, err := s.CheckSession(context.Background(), "session", actor, headerForACL()); err == nil {
		t.Fatal("old session pin overrode cleared Agent sandbox")
	}
	f.sandboxPin = ""
	r := requestJSON("POST", "/api/v1/agent-chat/session", `{"agent_id":"agent","query":"synthetic"}`)
	if err := s.Check(r.Context(), r, actor); err != nil {
		t.Fatal("safe reasoning disabled", err)
	}
}

func TestPlainChatChecksWorkspaceMemoryOnEveryCallButSearchDoesNot(t *testing.T) {
	s, f, store, actor := testScope()
	f.agent["config"] = safeAgentConfig()
	s.KnowledgeAccess = func(context.Context, string, http.Header) error { return nil }
	if err := store.Bind(context.Background(), "session", actor, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	request := func(path string) error {
		r := requestJSON("POST", path, `{"query":"synthetic","knowledge_base_ids":["a"]}`)
		return s.Check(r.Context(), r, actor)
	}
	if err := request("/api/v1/knowledge-chat/session"); err == nil {
		t.Fatal("plain chat ran without a workspace memory verifier")
	}
	enabled, checks := true, 0
	s.PlainChatAllowed = func(context.Context, http.Header) error {
		checks++
		if enabled {
			return &Error{403, "kb_acl.workspace_memory_disabled_required"}
		}
		return nil
	}
	if err := request("/api/v1/knowledge-chat/session"); err == nil {
		t.Fatal("plain chat inherited unsafe workspace memory")
	}
	if err := request("/api/v1/knowledge-search"); err != nil || checks != 1 {
		t.Fatal("pure retrieval unnecessarily depends on workspace memory", err, checks)
	}
	enabled = false
	if err := request("/api/v1/knowledge-chat/session"); err != nil || checks != 2 {
		t.Fatal("safe plain chat disabled or memory switch was cached", err, checks)
	}
	enabled = true
	r := requestJSON("POST", "/api/v1/knowledge-chat/session", `{"query":"synthetic","agent_id":"agent"}`)
	if err := s.Check(r.Context(), r, actor); err == nil || checks != 3 {
		t.Fatal("safe Agent could fall back to unsafe workspace memory after concurrent deletion", err, checks)
	}
}

func TestLegacyAgentNilMemoryCanOnlyInheritVerifiedDisabledWorkspace(t *testing.T) {
	s, f, _, actor := testScope()
	s.KnowledgeAccess = func(context.Context, string, http.Header) error { return nil }
	config := safeAgentConfig()
	delete(config, "memory_enabled")
	f.agent["config"] = config
	if _, err := s.Resolve(context.Background(), nil, false, "agent", actor, headerForACL()); err == nil {
		t.Fatal("nil memory inherited an unverified workspace switch")
	}
	s.PlainChatAllowed = func(context.Context, http.Header) error { return nil }
	if _, err := s.Resolve(context.Background(), nil, false, "agent", actor, headerForACL()); err != nil {
		t.Fatal("safe builtin/legacy Agent could not inherit a disabled workspace", err)
	}
}
