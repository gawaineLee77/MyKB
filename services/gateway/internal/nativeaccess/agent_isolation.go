package nativeaccess

import (
	"context"
	"net/http"
	"strings"
)

// Native chat can fall back to plain chat when an Agent is concurrently deleted
// or cannot be loaded. Every chat therefore checks the live workspace switch;
// pure search remains available even when legacy memory is enabled.
func (s *ScopeService) CheckPlainChat(ctx context.Context, h http.Header) error {
	if s.KnowledgeAccess == nil {
		return nil
	}
	if s.PlainChatAllowed == nil {
		return &Error{503, "kb_acl.memory_verification_unavailable"}
	}
	return s.PlainChatAllowed(ctx, h)
}

// Native built-in allowed_tools does not cover tools appended by the MCP,
// web-search or sandbox registrars. Keep those registrars disabled under ACL.
// Empty sandbox_config_id is the pinned native contract for no sandbox; there
// is no deployment-level fallback (tenant_sandbox_resolve.go).
func validateAgentIsolation(config map[string]any) error {
	// nil inherits the native workspace switch. Runtime callers independently
	// require that switch off; authoring below requires an explicit opt-out.
	if memory := config["memory_enabled"]; memory != nil && memory != false || config["web_search_enabled"] == true {
		return &Error{403, "agent.external_tools_disabled"}
	}
	quick := config["agent_mode"] == "quick-answer"
	tools := stringsValue(config["allowed_tools"])
	if !quick && len(tools) == 0 {
		return &Error{403, "agent.tools_scope_unavailable"}
	}
	for _, tool := range tools {
		if !scopedReadTools[tool] {
			return &Error{403, "agent.read_only_tools_required"}
		}
	}
	mode, _ := config["mcp_selection_mode"].(string)
	if mode != "none" && !(quick && mode == "") && !(mode == "selected" && len(stringsValue(config["mcp_services"])) == 0) {
		return &Error{403, "agent.mcp_scope_unavailable"}
	}
	if len(stringsValue(config["mcp_services"])) > 0 {
		return &Error{403, "agent.mcp_scope_unavailable"}
	}
	skills, _ := config["skills_selection_mode"].(string)
	if skills != "" && skills != "none" && skills != "selected" || len(stringsValue(config["selected_skills"])) > 0 {
		return &Error{403, "agent.skills_scope_unavailable"}
	}
	if sandbox, ok := config["sandbox_config_id"]; ok && sandbox != nil && sandbox != "" {
		return &Error{403, "agent.sandbox_scope_unavailable"}
	}
	return nil
}

func validateAgentRequestIsolation(body map[string]any) error {
	if body["web_search_enabled"] == true || len(stringsValue(body["mcp_service_ids"])) > 0 || len(stringsValue(body["skill_names"])) > 0 {
		return &Error{403, "agent.external_tools_disabled"}
	}
	if items, ok := body["mentioned_items"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok && (m["type"] == "mcp" || m["type"] == "skill") {
				return &Error{403, "agent.external_tools_disabled"}
			}
		}
	}
	return nil
}

// A native session pin takes priority over the current Agent sandbox setting.
// Reusing a pre-existing sandbox could therefore reactivate file tools after a
// safe Agent edit. Such sessions must be replaced under the ACL contract.
func (s *ScopeService) checkSessionIsolation(ctx context.Context, id string, h http.Header) error {
	value, err := s.data(ctx, "/api/v1/sessions/"+id, nil, h)
	if err != nil {
		return err
	}
	object, ok := value.(map[string]any)
	if !ok || object["id"] != id {
		return &Error{502, "upstream.invalid_response"}
	}
	if pinned, ok := object["sandbox_config_id"]; ok && pinned != nil && pinned != "" {
		return &Error{403, "session.sandbox_scope_unavailable"}
	}
	return nil
}

func (s *ScopeService) validateAgentMutation(ctx context.Context, r *http.Request, body map[string]any) error {
	path := r.URL.Path
	create := r.Method == "POST" && path == "/api/v1/agents"
	update := r.Method == "PUT" && strings.HasPrefix(path, "/api/v1/agents/")
	copyAgent := r.Method == "POST" && strings.HasPrefix(path, "/api/v1/agents/") && strings.HasSuffix(path, "/copy")
	if !create && !update && !copyAgent {
		return nil
	}
	config, provided := body["config"].(map[string]any)
	// Native copy ignores request configuration and clones the source instead.
	if !provided || copyAgent {
		if create {
			return &Error{400, "agent.safe_configuration_required"}
		}
		id := strings.TrimPrefix(path, "/api/v1/agents/")
		id = strings.TrimSuffix(id, "/copy")
		var err error
		config, err = s.AgentConfig(ctx, id, r.Header)
		if err != nil {
			return err
		}
	}
	if config["memory_enabled"] != false {
		return &Error{403, "agent.memory_opt_out_required"}
	}
	return validateAgentIsolation(config)
}
