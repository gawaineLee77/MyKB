package weknora

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// NativeRequest preserves native DTOs for product-generated scope/model/tool
// calls. Incoming arbitrary paths are handled separately by the exact manifest.
func (c *Client) NativeRequest(ctx context.Context, method, path string, query url.Values, h http.Header, input any) (json.RawMessage, error) {
	allowed := method == http.MethodGet && path == "/api/v1/system/info"
	for _, prefix := range []string{"/api/v1/knowledge-bases", "/api/v1/shared-knowledge-bases", "/api/v1/shared-agents", "/api/v1/knowledge/", "/api/v1/chunks/", "/api/v1/agents", "/api/v1/sessions", "/api/v1/models", "/api/v1/embed-channels"} {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/") {
			allowed = true
		}
	}
	if !allowed {
		return nil, &Error{Code: "upstream.request_invalid", StatusCode: 400}
	}
	var result json.RawMessage
	err := c.sendJSON(ctx, method, path, query, h, input, &result)
	return result, err
}
