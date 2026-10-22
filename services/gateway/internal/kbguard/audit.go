package kbguard

import (
	"net/http"
	"strings"
)

// AuditEvent contains identifiers and authorization decisions only. Route is
// a fixed template; request bodies, file names, content and credentials never
// enter this record. The callback resolves the actor from OriginalHeaders.
type AuditEvent struct {
	KBIDs     []string          `json:"kb_ids"`
	Actions   []string          `json:"actions"`
	Targets   []string          `json:"targets"`
	Revisions map[string]uint64 `json:"revisions"`
	Outcome   string            `json:"outcome"`
	Status    int               `json:"status"`
	Method    string            `json:"method"`
	Route     string            `json:"route"`
}

func auditEvent(r *http.Request, s *state) (AuditEvent, error) {
	e := AuditEvent{KBIDs: append([]string(nil), s.kbs...), Actions: append([]string(nil), s.route.actions...), Targets: []string{}, Revisions: map[string]uint64{}, Method: r.Method, Route: s.route.pattern}
	for kb, p := range s.permissions {
		e.Revisions[kb] = p.Revision
	}
	for _, key := range []string{"knowledge", "chunk", "entry", "slug", "folder", "tag"} {
		if id := s.params[key]; id != "" {
			e.Targets = append(e.Targets, key+":"+id)
		}
	}
	for _, key := range []string{"ids", "knowledge_ids", "knowledge_id", "chunk_ids", "chunk_id"} {
		for _, id := range stringList(s.body[key]) {
			e.Targets = append(e.Targets, key+":"+id)
		}
		if ids, err := sequenceList(s.body[key]); err == nil {
			for _, id := range ids {
				e.Targets = append(e.Targets, key+":"+id)
			}
		}
	}
	for _, key := range []string{"updates", "by_id", "by_tag"} {
		if entries, ok := s.body[key].(map[string]any); ok {
			for id := range entries {
				e.Targets = append(e.Targets, key+":"+id)
			}
		}
	}
	if slug, ok := s.body["slug"].(string); ok && slug != "" {
		e.Targets = append(e.Targets, "slug:"+slug)
	}
	if id := r.URL.Query().Get("knowledge_id"); id != "" {
		e.Targets = append(e.Targets, "knowledge:"+id)
	}
	e.Targets = unique(e.Targets)
	if len(e.Targets) > 200 {
		return e, fail(413, "kb_acl.too_many_targets")
	}
	for _, id := range e.Targets {
		if len(id) > 300 || strings.ContainsAny(id, "\x00\r\n") {
			return e, fail(400, "kb_acl.invalid_id")
		}
	}
	return e, nil
}
