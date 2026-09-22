package weknora

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// EnterpriseRequest preserves the pinned native DTOs at the product-owned
// account/workspace boundary. Paths are constructed by internal callers only.
func (c *Client) EnterpriseRequest(ctx context.Context, method, path string, h http.Header, input any) (json.RawMessage, error) {
	path, rawQuery, _ := strings.Cut(path, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, &Error{Code: "upstream.request_invalid", StatusCode: 400}
	}
	if !strings.HasPrefix(path, "/api/v1/auth/") && !strings.HasPrefix(path, "/api/v1/tenants") && !strings.HasPrefix(path, "/api/v1/system/admin/settings/") {
		return nil, &Error{Code: "upstream.request_invalid", StatusCode: 400}
	}
	var data json.RawMessage
	err = c.sendJSON(ctx, method, path, query, h, input, &data)
	return data, err
}
