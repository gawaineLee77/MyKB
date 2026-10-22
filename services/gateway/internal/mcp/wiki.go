package mcp

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type wikiKnowledge interface {
	WikiEnabled(context.Context, string, http.Header) (bool, error)
	SearchWikiPages(context.Context, string, string, int, http.Header) ([]weknora.WikiPage, error)
	GetWikiPage(context.Context, string, string, http.Header) (weknora.WikiPage, error)
}

type wikiHit struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	PageType        string `json:"page_type"`
	Status          string `json:"status"`
	Summary         string `json:"summary"`
	Snippet         string `json:"snippet"`
}

func (s *NativeService) callWiki(ctx context.Context, name string, raw json.RawMessage, actor nativeaccess.Actor, h http.Header) (result any, ids []string, err error) {
	reader, ok := s.Knowledge.(wikiKnowledge)
	// Unlike historical compatibility mode, these tools require the product
	// ACL callback. A native tenant projection alone is not a KB permission.
	if !ok || s.Scopes == nil || s.Scopes.KnowledgeAccess == nil || s.Scopes.Upstream == nil {
		return nil, nil, ErrUnavailable
	}
	if name == "wiki_read_page" {
		var input struct {
			KBID     string `json:"knowledge_base_id"`
			Slug     string `json:"slug"`
			Offset   int    `json:"offset"`
			MaxChars *int   `json:"max_chars"`
		}
		if err := decodeArguments(raw, &input); err != nil {
			return nil, nil, err
		}
		maxChars := 8000
		if input.MaxChars != nil {
			maxChars = *input.MaxChars
		}
		if !weknora.ValidWikiKBID(input.KBID) || !weknora.ValidWikiSlug(input.Slug) || input.Offset < 0 || input.Offset > 1000000 || maxChars < 1 || maxChars > 20000 {
			return nil, nil, ErrInvalid
		}
		ids = []string{input.KBID}
		if err := s.Scopes.CheckKB(ctx, input.KBID, h); err != nil {
			return nil, ids, err
		}
		page, err := reader.GetWikiPage(ctx, input.KBID, input.Slug, h)
		if err != nil {
			return nil, ids, err
		}
		if err := s.validateWikiPage(ctx, page, input.KBID, input.Slug, actor, h); err != nil {
			return nil, ids, err
		}
		content := []rune(wikiSafeText(page.Content))
		if input.Offset > len(content) {
			return nil, ids, ErrInvalid
		}
		end := min(input.Offset+maxChars, len(content))
		response := map[string]any{
			"knowledge_base_id": input.KBID, "slug": page.Slug, "title": truncate(wikiSafeText(page.Title), 512),
			"page_type": page.PageType, "status": page.Status, "summary": truncate(wikiSafeText(page.Summary), 2000),
			"content": string(content[input.Offset:end]), "offset": input.Offset, "truncated": end < len(content),
			"source_ids": wikiSourceIDs(page.SourceRefs), "chunk_ids": page.ChunkRefs,
		}
		if end < len(content) {
			response["next_offset"] = end
		}
		return response, ids, nil
	}
	var input struct {
		Query string   `json:"query"`
		KBIDs []string `json:"knowledge_base_ids"`
		Limit *int     `json:"limit"`
	}
	if err := decodeArguments(raw, &input); err != nil {
		return nil, nil, err
	}
	input.Query = strings.TrimSpace(input.Query)
	limit := 10
	if input.Limit != nil {
		limit = *input.Limit
	}
	if input.Query == "" || !utf8.ValidString(input.Query) || utf8.RuneCountInString(input.Query) > 4000 || limit < 1 || limit > 20 || len(input.KBIDs) > nativeaccess.MaxScope {
		return nil, nil, ErrInvalid
	}
	for _, id := range input.KBIDs {
		if !weknora.ValidWikiKBID(id) {
			return nil, nil, ErrInvalid
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, nil, ErrInvalid
	}
	// Omission selects the default scope; an explicitly empty/null scope must
	// stay empty and be rejected by Resolve, never expand to all accessible KBs.
	_, explicit := fields["knowledge_base_ids"]
	ids, err = s.Scopes.Resolve(ctx, input.KBIDs, explicit, "", actor, h)
	if err != nil {
		return nil, ids, err
	}
	hits := []wikiHit{}
	searched := []string{}
	truncated := false
	for _, kb := range ids {
		// Resolve validated the whole request before any retrieval. Recheck as
		// each KB is used, and again before returning any buffered response.
		if err := s.Scopes.CheckKB(ctx, kb, h); err != nil {
			return nil, ids, err
		}
		enabled, err := reader.WikiEnabled(ctx, kb, h)
		if err != nil {
			return nil, ids, err
		}
		if !enabled {
			continue
		}
		if err := s.Scopes.CheckKB(ctx, kb, h); err != nil {
			return nil, ids, err
		}
		pages, err := reader.SearchWikiPages(ctx, kb, input.Query, limit-len(hits), h)
		if err != nil {
			return nil, ids, err
		}
		if len(pages) > limit-len(hits) {
			return nil, ids, ErrUnavailable
		}
		searched = append(searched, kb)
		for _, page := range pages {
			if err := s.validateWikiPage(ctx, page, kb, "", actor, h); err != nil {
				return nil, ids, err
			}
			hits = append(hits, wikiHit{KnowledgeBaseID: kb, Slug: page.Slug, Title: truncate(wikiSafeText(page.Title), 512), PageType: page.PageType, Status: page.Status, Summary: truncate(wikiSafeText(page.Summary), 2000), Snippet: wikiSnippet(wikiSafeText(page.Content), input.Query)})
		}
		if len(hits) == limit {
			// A full final batch can have additional native matches even when
			// this was the last KB. Callers can narrow the KB or search text.
			truncated = true
			break
		}
	}
	for _, kb := range searched {
		if err := s.Scopes.CheckKB(ctx, kb, h); err != nil {
			return nil, ids, err
		}
	}
	return map[string]any{"knowledge_base_ids": ids, "effective_scope": ids, "searched_knowledge_base_ids": searched, "results": hits, "truncated": truncated}, ids, nil
}

func (s *NativeService) validateWikiPage(ctx context.Context, page weknora.WikiPage, kb, slug string, actor nativeaccess.Actor, h http.Header) error {
	if page.ID == "" || page.KnowledgeBaseID != kb || page.TenantID != actor.TenantID || !weknora.ValidWikiSlug(page.Slug) || slug != "" && page.Slug != slug || len(page.SourceRefs)+len(page.ChunkRefs) > 256 {
		return ErrDenied
	}
	if !weknora.ValidWikiPageType(page.PageType) || page.Status != "draft" && page.Status != "published" && page.Status != "archived" {
		return ErrUnavailable
	}
	for _, id := range wikiSourceIDs(page.SourceRefs) {
		if !weknora.ValidWikiKBID(id) {
			return ErrDenied
		}
		parent, err := s.Scopes.Upstream.KnowledgeBaseForKnowledge(ctx, id, h)
		if err != nil {
			return err
		}
		if parent != kb {
			return ErrDenied
		}
	}
	for _, id := range page.ChunkRefs {
		if !weknora.ValidWikiKBID(id) {
			return ErrDenied
		}
		parent, err := s.Scopes.Upstream.KnowledgeBaseForChunk(ctx, id, h)
		if err != nil {
			return err
		}
		if parent != kb {
			return ErrDenied
		}
	}
	return s.Scopes.CheckKB(ctx, kb, h)
}

func wikiSourceIDs(refs []string) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		id, _, _ := strings.Cut(ref, "|")
		ids = append(ids, id)
	}
	return ids
}

var wikiURL = regexp.MustCompile(`(?i)(?:(?:https?|resource|local|minio|s3|oss|cos|tos|storage|file)(?:://|%3a%2f%2f)|/api/v1/(?:resources|knowledge|chunks)/)[^\s<>"'\x60\[\]()]+`)
var wikiSignature = regexp.MustCompile(`(?i)(?:x-amz-signature|x-goog-signature|x-oss-signature|q-signature|x-tos-signature|ossaccesskeyid|awsaccesskeyid|access_token|api_key|signature|sig|token|credential)=[^\s<>"'\x60\[\]()]+`)

// MCP reads text only. Never return a reusable object-store locator, signed
// URL or a resource handle that has not established its own parent KB.
func wikiSafeText(value string) string {
	value = wikiURL.ReplaceAllStringFunc(value, func(link string) string {
		if wikiSensitiveURL(link) {
			return "[resource redacted]"
		}
		return link
	})
	return wikiSignature.ReplaceAllString(value, "[resource redacted]")
}

func wikiSensitiveURL(link string) bool {
	// Decode only for inspection, never for forwarding. HTML and URL escaping
	// cannot hide SAS, cloud signatures or private resource paths from this check.
	decoded := html.UnescapeString(link)
	for i := 0; i < 3; i++ {
		next, err := url.QueryUnescape(decoded)
		if err != nil {
			return true
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	u, err := url.Parse(decoded)
	if err != nil || u.User != nil || u.Scheme != "http" && u.Scheme != "https" || wikiSignature.MatchString(decoded) {
		return true
	}
	lowerPath := strings.ToLower(u.Path)
	for _, privatePath := range []string{"/api/", "/uploads/", "/storage/", "/attachments/"} {
		if strings.Contains(lowerPath, privatePath) {
			return true
		}
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return true
	}
	for key := range query {
		key = strings.ToLower(key)
		for _, prefix := range []string{"x-amz-", "x-goog-", "x-oss-", "x-tos-", "q-sign"} {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
		switch key {
		case "sig", "signature", "token", "access_token", "auth", "authorization", "api_key", "apikey", "key", "secret", "password", "credential", "awsaccesskeyid", "ossaccesskeyid", "session", "session_id", "jwt", "sas":
			return true
		}
	}
	return false
}

func wikiSnippet(content, query string) string {
	runes := []rune(content)
	start := 0
	if at := strings.Index(strings.ToLower(content), strings.ToLower(query)); at >= 0 {
		start = max(0, utf8.RuneCountInString(strings.ToLower(content)[:at])-160)
	}
	return string(runes[start:min(start+800, len(runes))])
}

func wikiToolDefinitions() []map[string]any {
	readOnly := map[string]bool{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false}
	return []map[string]any{
		{"name": "wiki_search", "description": "Search generated Wiki pages using literal text within currently authorized KBs. Returns bounded summaries and snippets; use wiki_read_page for the body. Wiki-disabled KBs are skipped. A truncated result can be refined by KB or query.", "inputSchema": objectSchema(map[string]any{"query": map[string]any{"type": "string", "minLength": 1, "maxLength": 4000}, "knowledge_base_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string", "maxLength": 128}, "maxItems": nativeaccess.MaxScope}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}}, []string{"query"}), "annotations": readOnly},
		{"name": "wiki_read_page", "description": "Read a bounded Markdown body from an authorized Wiki page identified by KB and slug. Follow next_offset for remaining text. Resource URLs are redacted; this does not grant original-document download access.", "inputSchema": objectSchema(map[string]any{"knowledge_base_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "slug": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}, "offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000000, "default": 0}, "max_chars": map[string]any{"type": "integer", "minimum": 1, "maximum": 20000, "default": 8000}}, []string{"knowledge_base_id", "slug"}), "annotations": readOnly},
	}
}
