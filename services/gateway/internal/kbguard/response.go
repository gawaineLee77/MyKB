package kbguard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
)

// FilterResponse runs before reverseproxy emits response headers or bytes.
func (g *Guard) FilterResponse(response *http.Response) error {
	s, _ := response.Request.Context().Value(stateKey{}).(*state)
	if s == nil || response.StatusCode < 200 {
		return nil
	}
	s.responsePermissions = map[string]kbacl.Permissions{}
	response.Header.Set("Cache-Control", "private, no-store")
	response.Header.Del("Expires")
	if s.route.kind == "delete" && response.StatusCode < 300 {
		if g.MarkDeleted == nil {
			return fail(503, "kb_acl.tombstone_unavailable")
		}
		if err := g.MarkDeleted(response.Request.Context(), s.headers, s.params["kb"]); err != nil {
			return err
		}
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "json") {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
	_ = response.Body.Close()
	if err != nil || len(raw) > 16<<20 {
		return fail(502, "kb_acl.response_too_large")
	}
	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		return fail(502, "kb_acl.invalid_upstream_response")
	}
	if s.route.kind == "create" && response.StatusCode < 300 && envelope["success"] == true {
		data, ok := envelope["data"].(map[string]any)
		if !ok {
			return fail(502, "kb_acl.invalid_upstream_response")
		}
		id, _ := data["id"].(string)
		if !validID(id) {
			return fail(502, "kb_acl.invalid_upstream_response")
		}
		if g.AfterCreate == nil {
			return fail(503, "kb_acl.creation_binding_unavailable")
		}
		if err := g.AfterCreate(response.Request.Context(), s.headers, s.createKey, id); err != nil {
			return err
		}
		s.kbs = []string{id}
	}
	if s.route.kind == "list" && response.StatusCode < 300 {
		rows, ok := envelope["data"].([]any)
		if !ok && envelope["data"] != nil {
			return fail(502, "kb_acl.invalid_upstream_response")
		}
		ids := []string{}
		for _, row := range rows {
			obj, ok := row.(map[string]any)
			if !ok {
				return fail(502, "kb_acl.invalid_upstream_response")
			}
			id, _ := obj["id"].(string)
			if !validID(id) {
				return fail(502, "kb_acl.invalid_upstream_response")
			}
			ids = append(ids, id)
		}
		allowed, e := g.ACL.AllowedKnowledgeBases(response.Request.Context(), s.headers, ids)
		if e != nil {
			return e
		}
		visible := []any{}
		for _, row := range rows {
			obj := row.(map[string]any)
			id := obj["id"].(string)
			if contains(allowed, id) {
				visible = append(visible, obj)
			}
		}
		envelope["data"] = visible
		envelope["total"] = len(visible)
		envelope["has_more"] = false
	}
	value := envelope["data"]
	if s.route.kind == "wiki" {
		value = envelope
	}
	if g.RewriteResources != nil {
		if err := g.RewriteResources(response.Request.Context(), s.headers, value); err != nil {
			return err
		}
	}
	if err := g.filterValue(response.Request, s, value, ""); err != nil {
		return err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	return nil
}

func (g *Guard) filterValue(r *http.Request, s *state, value any, parentKB string) error {
	switch v := value.(type) {
	case []any:
		for _, row := range v {
			if err := g.filterValue(r, s, row, parentKB); err != nil {
				return err
			}
		}
	case map[string]any:
		kb := parentKB
		if id, ok := v["knowledge_base_id"].(string); ok && id != "" {
			kb = id
		}
		if id, ok := v["kb_id"].(string); ok && id != "" {
			kb = id
		}
		if kb == "" && len(s.kbs) == 1 {
			kb = s.kbs[0]
		}
		if s.route.kind == "list" && parentKB == "" {
			if id, ok := v["id"].(string); ok && v["knowledge_base_id"] == nil && v["knowledge_id"] == nil {
				kb = id
			}
		}
		var permission kbacl.Permissions
		if kb != "" {
			if len(s.kbs) > 0 && s.route.kind != "list" && !contains(s.kbs, kb) {
				return fail(502, "kb_acl.upstream_parent_mismatch")
			}
			if s.responsePermissions == nil {
				s.responsePermissions = map[string]kbacl.Permissions{}
			}
			p, ok := s.responsePermissions[kb]
			if !ok {
				var err error
				p, err = g.ACL.Authorize(r.Context(), s.headers, kb)
				if err != nil {
					return err
				}
				s.responsePermissions[kb] = p
			}
			permission = p
		}
		download := permission.Manage || contains(permission.Operations, "document.download")
		// A Wiki graph edge's source is a page slug, not an original-file
		// locator. Only redact the native knowledge object's source field.
		_, hasFileName := v["file_name"]
		_, hasFilePath := v["file_path"]
		_, hasParseStatus := v["parse_status"]
		originalDocument := hasFileName || hasFilePath || hasParseStatus
		for key, item := range v {
			if sensitiveField(key) || (!download && key == "source" && originalDocument) {
				delete(v, key)
				continue
			}
			if text, ok := item.(string); ok {
				if key == "image_info" {
					var data any
					if json.Unmarshal([]byte(text), &data) == nil {
						scrubResourceFields(data)
						encoded, _ := json.Marshal(data)
						v[key] = string(encoded)
					}
				} else if storageURL(text) {
					delete(v, key)
				}
				continue
			}
			if err := g.filterValue(r, s, item, kb); err != nil {
				return err
			}
		}
	}
	return nil
}
func sensitiveField(key string) bool {
	switch strings.ToLower(key) {
	case "file_path", "file_url", "download_url", "preview_url", "presigned_url", "signed_url", "storage_url", "original_url", "physical_path", "failed_entries_url":
		return true
	}
	return false
}
func storageURL(v string) bool {
	for _, prefix := range []string{"local://", "minio://", "s3://", "oss://", "cos://", "tos://", "storage://", "file://"} {
		if strings.Contains(v, prefix) {
			return true
		}
	}
	lower := strings.ToLower(v)
	return strings.Contains(lower, "x-amz-signature=") || strings.Contains(lower, "x-oss-signature=") || strings.Contains(lower, "q-signature=")
}
func scrubResourceFields(value any) {
	switch v := value.(type) {
	case []any:
		for _, v := range v {
			scrubResourceFields(v)
		}
	case map[string]any:
		for key, item := range v {
			if sensitiveField(key) {
				delete(v, key)
				continue
			}
			if text, ok := item.(string); ok && storageURL(text) {
				delete(v, key)
			} else {
				scrubResourceFields(item)
			}
		}
	}
}
