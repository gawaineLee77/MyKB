package nativeaccess

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Native organization sidebars aggregate resources without a product KB ACL
// predicate. Their counts must not disclose otherwise invisible knowledge.
func (s *ScopeService) FilterWorkspaceMetadata(r *http.Response) error {
	if s.KnowledgeAccess == nil || r.StatusCode < 200 || r.StatusCode >= 300 || r.Request.Method != "GET" {
		return nil
	}
	path := r.Request.URL.Path
	if !(path == "/api/v1/organizations" || strings.HasPrefix(path, "/api/v1/organizations/") || path == "/api/v1/auth/me") {
		return nil
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "json") {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<20+1))
	r.Body.Close()
	if err != nil || len(raw) > 16<<20 {
		return &Error{502, "upstream.response_too_large"}
	}
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&value) != nil {
		return &Error{502, "upstream.invalid_response"}
	}
	var scrub func(any)
	scrub = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, child := range x {
				scrub(child)
			}
		case map[string]any:
			for k, child := range x {
				switch k {
				case "resource_counts", "knowledge_base_count", "knowledge_count", "agent_count":
					delete(x, k)
				default:
					scrub(child)
				}
			}
		}
	}
	scrub(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(encoded))
	r.ContentLength = int64(len(encoded))
	r.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	r.Header.Set("Cache-Control", "private, no-store")
	return nil
}
