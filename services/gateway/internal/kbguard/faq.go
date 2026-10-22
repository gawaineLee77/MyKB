package kbguard

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// FAQ identifiers are native sequence numbers, not knowledge UUIDs. Resolve
// every explicit entry through its KB endpoint before accepting a batch.
func (g *Guard) validateFAQ(r *http.Request, s *state) error {
	kb := s.params["kb"]
	var ids, tags []string
	if id := s.params["entry"]; id != "" {
		ids = append(ids, id)
	}
	if _, present := s.body["task_id"]; present {
		return fail(400, "kb_acl.task_id_immutable")
	}
	for _, key := range []string{"ids", "exclude_ids"} {
		values, err := sequenceList(s.body[key])
		if err != nil {
			return err
		}
		ids = append(ids, values...)
	}
	for _, key := range []string{"updates", "by_id", "by_tag"} {
		if items, ok := s.body[key].(map[string]any); ok {
			for id, value := range items {
				if _, err := sequence(id); err != nil {
					return err
				}
				if key == "by_tag" {
					tags = append(tags, id)
				} else {
					ids = append(ids, id)
				}
				if key == "updates" {
					if value != nil {
						tag, err := sequence(value)
						if err != nil {
							return err
						}
						if tag != "0" {
							tags = append(tags, tag)
						}
					}
				} else if fields, ok := value.(map[string]any); ok {
					for field := range fields {
						if field != "tag_id" && field != "is_enabled" && field != "is_recommended" {
							return fail(400, "kb_acl.field_not_allowed")
						}
					}
					if value := fields["tag_id"]; value != nil {
						tag, err := sequence(value)
						if err != nil {
							return err
						}
						if tag != "0" {
							tags = append(tags, tag)
						}
					}
				} else {
					return fail(400, "kb_acl.invalid_json")
				}
			}
		}
	}
	entries := []map[string]any{s.body}
	if items, ok := s.body["entries"].([]any); ok {
		if len(items) == 0 || len(items) > 1000 {
			return fail(400, "kb_acl.faq_batch_size")
		}
		entries = nil
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return fail(400, "kb_acl.invalid_json")
			}
			entries = append(entries, entry)
		}
	}
	for _, entry := range entries {
		if value, ok := entry["id"]; ok {
			id, err := sequence(value)
			if err != nil || id == "0" {
				return fail(400, "kb_acl.invalid_id")
			}
			ids = append(ids, id)
		}
		if value := entry["tag_id"]; value != nil {
			id, err := sequence(value)
			if err != nil {
				return err
			}
			if id != "0" {
				tags = append(tags, id)
			}
		}
		if !s.permissions[kb].Manage {
			if name, ok := entry["tag_name"].(string); ok && strings.TrimSpace(name) != "" {
				return fail(403, "kb_acl.tag_management_required")
			}
			if entry != nil && s.body["entries"] != nil {
				allowed := strings.Fields("id standard_question similar_questions negative_questions answers answer_strategy tag_id tag_name is_enabled is_recommended")
				for key := range entry {
					if !contains(allowed, key) {
						return fail(400, "kb_acl.field_not_allowed")
					}
				}
			}
		}
	}
	if len(unique(ids)) > 1000 {
		return fail(413, "kb_acl.too_many_targets")
	}
	for _, id := range unique(ids) {
		if id == "0" {
			return fail(400, "kb_acl.invalid_id")
		}
		entry, err := g.object(r.Context(), "/api/v1/knowledge-bases/"+url.PathEscape(kb)+"/faq/entries/"+url.PathEscape(id), nil, s.headers)
		if err != nil {
			return err
		}
		if entry["knowledge_base_id"] != kb {
			return fail(403, "kb_acl.parent_mismatch")
		}
	}
	if len(unique(tags)) > 0 {
		e, err := g.envelope(r.Context(), "/api/v1/knowledge-bases/"+url.PathEscape(kb)+"/tags", nil, s.headers)
		if err != nil {
			return err
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
				if id, err := sequence(x["seq_id"]); err == nil {
					allowed[id] = true
				}
				for _, v := range x {
					scan(v)
				}
			}
		}
		scan(e["data"])
		for _, id := range unique(tags) {
			if !allowed[id] {
				return fail(403, "kb_acl.tag_parent_mismatch")
			}
		}
	}
	return nil
}
func sequence(v any) (string, error) {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case json.Number:
		s = x.String()
	case float64:
		if x != float64(int64(x)) {
			return "", fail(400, "kb_acl.invalid_id")
		}
		s = strconv.FormatInt(int64(x), 10)
	default:
		return "", fail(400, "kb_acl.invalid_id")
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 0 {
		return "", fail(400, "kb_acl.invalid_id")
	}
	return strconv.FormatInt(n, 10), nil
}
func sequenceList(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fail(400, "kb_acl.invalid_id")
	}
	out := []string{}
	for _, v := range list {
		id, e := sequence(v)
		if e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, nil
}
