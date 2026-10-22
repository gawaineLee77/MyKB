package kbguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// VerifyResource deliberately refuses a claim made only by editable Markdown
// or image_info. Such a claim would let a collaborator launder a hidden KB's
// resource handle through a visible document. Integration must provide a
// trusted server-side resource binding resolver through AuthorizeResource.
func (g *Guard) VerifyResource(context.Context, http.Header, string, string) error {
	return fail(409, "kb_acl.resource_binding_unavailable")
}

func (g *Guard) paged(ctx context.Context, path string, q url.Values, h http.Header, max int) ([]any, error) {
	if q == nil {
		q = url.Values{}
	}
	rows := []any{}
	for page := 1; page <= max/100+1; page++ {
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", "100")
		e, err := g.envelope(ctx, path, q, h)
		if err != nil {
			return nil, err
		}
		items, ok := e["data"].([]any)
		if !ok && e["data"] != nil {
			return nil, fail(502, "kb_acl.invalid_upstream_response")
		}
		rows = append(rows, items...)
		if len(rows) > max {
			return nil, fail(422, "kb_acl.collection_too_large")
		}
		total, hasTotal := number(e["total"])
		if len(items) == 0 || (hasTotal && len(rows) >= total) || (!hasTotal && len(items) < 100) {
			return rows, nil
		}
	}
	return nil, fail(422, "kb_acl.collection_too_large")
}

func (g *Guard) search(w http.ResponseWriter, r *http.Request, s *state) error {
	// Native global search cannot take a human KB allow-list. Query only
	// authorized KBs through their list endpoint, then paginate that union.
	if r.URL.Query().Get("agent_id") != "" || r.URL.Query().Get("agent_source_tenant_id") != "" {
		return fail(409, "kb_acl.agent_search_not_supported")
	}
	listed, err := g.envelope(r.Context(), "/api/v1/knowledge-bases", nil, s.headers)
	if err != nil {
		return err
	}
	items, ok := listed["data"].([]any)
	if !ok && listed["data"] != nil {
		return fail(502, "kb_acl.invalid_upstream_response")
	}
	ids := []string{}
	for _, v := range items {
		row, ok := v.(map[string]any)
		if !ok {
			return fail(502, "kb_acl.invalid_upstream_response")
		}
		id, _ := row["id"].(string)
		if !validID(id) {
			return fail(502, "kb_acl.invalid_upstream_response")
		}
		ids = append(ids, id)
	}
	ids, err = g.ACL.AllowedKnowledgeBases(r.Context(), s.headers, ids)
	if err != nil {
		return err
	}
	if len(ids) > 256 {
		return fail(422, "kb_acl.collection_too_large")
	}
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		keyword = r.URL.Query().Get("query")
	}
	rows := []any{}
	for _, kb := range ids {
		if _, err := g.ACL.Authorize(r.Context(), s.headers, kb); err != nil {
			return err
		}
		q := url.Values{}
		if keyword != "" {
			q.Set("keyword", keyword)
		}
		docs, err := g.paged(r.Context(), "/api/v1/knowledge-bases/"+url.PathEscape(kb)+"/knowledge", q, s.headers, 5000)
		if err != nil {
			return err
		}
		for _, doc := range docs {
			obj, ok := doc.(map[string]any)
			if !ok || obj["knowledge_base_id"] != kb {
				return fail(502, "kb_acl.upstream_parent_mismatch")
			}
			if filter := r.URL.Query().Get("file_types"); filter != "" {
				kind, _ := obj["file_type"].(string)
				if !contains(strings.Split(filter, ","), kind) {
					continue
				}
			}
			rows = append(rows, obj)
			if len(rows) > 10000 {
				return fail(422, "kb_acl.collection_too_large")
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, _ := rows[i].(map[string]any)["created_at"].(string)
		b, _ := rows[j].(map[string]any)["created_at"].(string)
		if a == b {
			ai, _ := rows[i].(map[string]any)["id"].(string)
			bi, _ := rows[j].(map[string]any)["id"].(string)
			return ai < bi
		}
		return a > b
	})
	offset, err := positiveQuery(r.URL.Query(), "offset", 0, 10000)
	if err != nil {
		return err
	}
	limit, err := positiveQuery(r.URL.Query(), "limit", 20, 100)
	if err != nil {
		return err
	}
	if limit == 0 {
		limit = 20
	}
	total := len(rows)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	rows = rows[offset:end]
	if err := g.filterValue(r, s, rows, ""); err != nil {
		return err
	}
	writeJSON(w, map[string]any{"success": true, "data": rows, "total": total, "has_more": end < total})
	return nil
}

func (g *Guard) export(w http.ResponseWriter, r *http.Request, s *state) error {
	ids := stringList(s.body["knowledge_ids"])
	ids = append(ids, stringList(s.body["knowledge_id"])...)
	if id := r.URL.Query().Get("knowledge_id"); id != "" {
		ids = append(ids, id)
	}
	chunks := []any{}
	for _, id := range unique(ids) {
		// Re-check on each page-bearing document before gathering any content.
		if _, err := g.ACL.Authorize(r.Context(), s.headers, s.params["kb"], "chunk.export"); err != nil {
			return err
		}
		rows, err := g.paged(r.Context(), "/api/v1/chunks/"+url.PathEscape(id), nil, s.headers, 10000)
		if err != nil {
			return err
		}
		for _, v := range rows {
			row, ok := v.(map[string]any)
			if !ok || row["knowledge_id"] != id || row["knowledge_base_id"] != s.params["kb"] {
				return fail(502, "kb_acl.upstream_parent_mismatch")
			}
			chunks = append(chunks, row)
			if len(chunks) > 10000 {
				return fail(422, "kb_acl.collection_too_large")
			}
		}
	}
	if err := g.filterValue(r, s, chunks, s.params["kb"]); err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", `attachment; filename="chunks.json"`)
	writeJSON(w, map[string]any{"knowledge_base_id": s.params["kb"], "chunks": chunks})
	return nil
}
func number(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), x >= 0
	case json.Number:
		n, e := strconv.Atoi(x.String())
		return n, e == nil && n >= 0
	case int:
		return x, x >= 0
	}
	return 0, false
}
func positiveQuery(q url.Values, key string, fallback, max int) (int, error) {
	s := q.Get(key)
	if s == "" {
		return fallback, nil
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 0 || n > max {
		return 0, fail(400, "kb_acl.pagination_invalid")
	}
	return n, nil
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(value)
}
