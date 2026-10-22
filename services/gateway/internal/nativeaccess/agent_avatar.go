package nativeaccess

import (
	"context"
	"net/http"
	"strings"
)

// AgentAvatarForRead uses the same tenant and KB visibility rules as Agent
// metadata. A short avatar reference never grants access to a hidden Agent.
func (s *ScopeService) AgentAvatarForRead(ctx context.Context, id string, h http.Header) (string, error) {
	if !validID(id) {
		return "", &Error{400, "scope.invalid_id"}
	}
	resolver, ok := s.Upstream.(PrincipalResolver)
	if !ok {
		return "", &Error{503, "identity.unavailable"}
	}
	actor, err := ResolveActor(ctx, resolver, h)
	if err != nil {
		return "", err
	}
	value, err := s.agent(ctx, id, h)
	if err != nil {
		return "", err
	}
	a, ok := value.(map[string]any)
	if !ok || a["id"] != id {
		return "", &Error{502, "upstream.invalid_response"}
	}
	if uintValue(a["tenant_id"]) != actor.TenantID {
		return "", &Error{404, "agent.not_found"}
	}
	config, ok := a["config"].(map[string]any)
	if !ok {
		return "", &Error{502, "upstream.invalid_response"}
	}
	for _, kb := range stringsValue(config["knowledge_bases"]) {
		if err := s.CheckKB(ctx, kb, h); err != nil {
			return "", err
		}
	}
	avatar, _ := a["avatar"].(string)
	return avatar, nil
}

func (s *ScopeService) validateAgentAvatarMutation(ctx context.Context, r *http.Request, body map[string]any) error {
	path := r.URL.Path
	create := r.Method == "POST" && path == "/api/v1/agents"
	update := r.Method == "PUT" && strings.HasPrefix(path, "/api/v1/agents/")
	copyAgent := r.Method == "POST" && strings.HasPrefix(path, "/api/v1/agents/") && strings.HasSuffix(path, "/copy")
	if !create && !update && !copyAgent {
		return nil
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/agents/"), "/copy")
	if create {
		id = ""
	}
	avatar, provided := body["avatar"]
	if copyAgent {
		value, err := s.agent(ctx, id, r.Header)
		if err != nil {
			return err
		}
		agent, ok := value.(map[string]any)
		if !ok {
			return &Error{502, "upstream.invalid_response"}
		}
		avatar, _ = agent["avatar"].(string)
		if text, _ := avatar.(string); strings.HasPrefix(text, "mc-avatar") {
			avatar, err = s.AgentAvatarForRead(ctx, id, r.Header)
			if err != nil {
				return err
			}
		}
		provided = true
	}
	if !provided {
		return nil
	}
	text, ok := avatar.(string)
	if !ok || len([]rune(text)) > 64 {
		return &Error{400, "agent_icon.reference_invalid"}
	}
	if s.AgentAvatarCheck != nil {
		return s.AgentAvatarCheck(ctx, r.Header, id, text)
	}
	if strings.HasPrefix(text, "mc-avatar") {
		return &Error{503, "agent_icon.unavailable"}
	}
	return nil
}
