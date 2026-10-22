package kbguard

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

func readBody(r *http.Request, rt route) (map[string]any, error) {
	body := map[string]any{}
	if r.Body == nil {
		return body, nil
	}
	limit := int64(16 << 20)
	if rt.kind == "upload" {
		limit = 64 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || int64(len(raw)) > limit {
		return nil, fail(413, "kb_acl.body_too_large")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return body, nil
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, fail(400, "kb_acl.content_type_invalid")
	}
	if rt.kind == "upload" {
		if media != "multipart/form-data" || params["boundary"] == "" {
			return nil, fail(400, "kb_acl.multipart_required")
		}
		reader := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
		files := 0
		fields := 0
		for {
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, fail(400, "kb_acl.multipart_invalid")
			}
			fields++
			if fields > 32 {
				return nil, fail(400, "kb_acl.multipart_invalid")
			}
			name := part.FormName()
			if name == "" || body[name] != nil {
				return nil, fail(400, "kb_acl.duplicate_field")
			}
			if part.FileName() != "" {
				if name != "file" {
					return nil, fail(400, "kb_acl.upload_field_denied")
				}
				files++
				body[name] = true
				_ = part.Close()
				continue
			}
			value, e := io.ReadAll(io.LimitReader(part, 1<<20+1))
			_ = part.Close()
			if e != nil || len(value) > 1<<20 {
				return nil, fail(400, "kb_acl.multipart_invalid")
			}
			body[name] = string(value)
		}
		if files != 1 {
			return nil, fail(400, "kb_acl.upload_file_required")
		}
		return body, nil
	}
	if media != "application/json" {
		return nil, fail(400, "kb_acl.json_required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil || body == nil {
		return nil, fail(400, "kb_acl.invalid_json")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fail(400, "kb_acl.invalid_json")
	}
	return body, nil
}

func forgedIdentity(body map[string]any) bool {
	for _, key := range []string{"creator_id", "created_by", "user_id", "owner_id", "tenant_id", "id", "visibility", "permissions", "members"} {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}
func validateFields(s *state) error {
	allowed := map[string]bool{}
	for _, key := range s.route.fields {
		allowed[key] = true
	}
	for key := range s.body {
		if !allowed[key] {
			return fail(400, "kb_acl.field_not_allowed")
		}
	}
	// Free-form parse overrides can reconfigure full pipelines. Collaborators
	// may use the established KB configuration, but cannot mutate that contract.
	if value, ok := s.body["process_config"]; ok && value != nil {
		if text, ok := value.(string); !ok || strings.TrimSpace(text) != "" {
			return fail(403, "kb_acl.parse_config_management_required")
		}
	}
	return nil
}

func (g *Guard) validateReferences(r *http.Request, s *state) error {
	// Verify every explicit nested content ID even on routes that natively only
	// check the URL KB. Unknown input fields cannot smuggle a second parent.
	var knowledge, chunks, kbs, tags []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, v := range x {
				walk(v)
			}
		case map[string]any:
			for key, v := range x {
				switch key {
				case "knowledge_id", "knowledge_ids":
					knowledge = append(knowledge, stringList(v)...)
				case "source_refs":
					for _, ref := range stringList(v) {
						knowledge = append(knowledge, strings.SplitN(ref, "|", 2)[0])
					}
				case "chunk_id", "chunk_ids", "chunk_refs":
					chunks = append(chunks, stringList(v)...)
				case "knowledge_base_id", "knowledge_base_ids", "kb_id", "kb_ids", "source_kb_id", "target_kb_id", "source_id", "target_id":
					kbs = append(kbs, stringList(v)...)
				case "tag_id", "tag_ids":
					tags = append(tags, stringList(v)...)
				}
				walk(v)
			}
		}
	}
	walk(s.body)
	for key, values := range r.URL.Query() {
		for _, value := range values {
			if key == "tag_ids" {
				tags = append(tags, strings.Split(value, ",")...)
			} else {
				walk(map[string]any{key: value})
			}
		}
	}
	if s.route.kind == "tags" {
		if updates, ok := s.body["updates"].(map[string]any); ok {
			for _, v := range updates {
				tags = append(tags, stringList(v)...)
			}
		}
	}
	if raw, ok := s.body["tag_ids"].(string); ok {
		tags = append(tags, strings.Split(raw, ",")...)
	}
	if len(unique(knowledge))+len(unique(chunks))+len(unique(kbs))+len(unique(tags)) > 1000 {
		return fail(413, "kb_acl.too_many_targets")
	}
	for _, id := range unique(knowledge) {
		if !validID(id) {
			return fail(400, "kb_acl.invalid_id")
		}
		kb, e := g.Upstream.KnowledgeBaseForKnowledge(r.Context(), id, s.headers)
		if e != nil {
			return e
		}
		kbs = append(kbs, kb)
	}
	for _, id := range unique(chunks) {
		if !validID(id) {
			return fail(400, "kb_acl.invalid_id")
		}
		kb, e := g.Upstream.KnowledgeBaseForChunk(r.Context(), id, s.headers)
		if e != nil {
			return e
		}
		kbs = append(kbs, kb)
	}
	for _, id := range unique(kbs) {
		if !validID(id) {
			return fail(400, "kb_acl.invalid_id")
		}
		if len(s.kbs) > 0 && !contains(s.kbs, id) {
			return fail(403, "kb_acl.parent_mismatch")
		}
		if _, e := g.ACL.Authorize(r.Context(), s.headers, id, s.route.actions...); e != nil {
			return e
		}
	}
	if len(unique(tags)) > 0 {
		if len(s.kbs) != 1 {
			return fail(403, "kb_acl.tag_scope_required")
		}
		envelope, e := g.envelope(r.Context(), "/api/v1/knowledge-bases/"+url.PathEscape(s.kbs[0])+"/tags", nil, s.headers)
		if e != nil {
			return e
		}
		allowed := map[string]bool{}
		var scan func(any)
		scan = func(v any) {
			switch x := v.(type) {
			case []any:
				for _, v := range x {
					scan(v)
				}
			case map[string]any:
				if id, ok := x["id"].(string); ok {
					allowed[id] = true
				}
				for _, v := range x {
					scan(v)
				}
			}
		}
		scan(envelope["data"])
		for _, id := range unique(tags) {
			if !allowed[id] {
				return fail(403, "kb_acl.tag_parent_mismatch")
			}
		}
	}
	return nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
