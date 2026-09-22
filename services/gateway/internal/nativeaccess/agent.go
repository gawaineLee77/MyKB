package nativeaccess

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// Native GET /agents/:id returns tenant-local agents only. The public shared
// list includes the authorized Agent config; query it with the same credential.
func (s *ScopeService) agent(ctx context.Context, id string, h http.Header) (any, error) {
	value, err := s.data(ctx, "/api/v1/agents/"+url.PathEscape(id), nil, h)
	if err == nil {
		return value, nil
	}
	var nativeErr *weknora.Error
	if !errors.As(err, &nativeErr) || nativeErr.StatusCode != 404 {
		return nil, err
	}
	missing := err
	value, err = s.data(ctx, "/api/v1/shared-agents", nil, h)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, missing
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, &Error{502, "upstream.invalid_response"}
	}
	var selected map[string]any
	for _, row := range rows {
		share, ok := row.(map[string]any)
		if !ok {
			continue
		}
		agent, ok := share["agent"].(map[string]any)
		if !ok || agent["id"] != id {
			continue
		}
		tenant := uintValue(share["source_tenant_id"])
		if tenant == 0 || tenant != uintValue(agent["tenant_id"]) {
			return nil, &Error{502, "upstream.invalid_response"}
		}
		if selected != nil && uintValue(selected["tenant_id"]) != tenant {
			return nil, &Error{409, "agent.source_ambiguous"}
		}
		selected = agent
	}
	if selected == nil {
		return nil, missing
	}
	return selected, nil
}
