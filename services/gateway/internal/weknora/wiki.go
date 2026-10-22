package weknora

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WikiPage is the read-only v0.8.0 contract. Resource URLs and unrelated native
// metadata are deliberately not part of this adapter's response model.
type WikiPage struct {
	ID              string   `json:"id"`
	TenantID        uint64   `json:"tenant_id"`
	KnowledgeBaseID string   `json:"knowledge_base_id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	PageType        string   `json:"page_type"`
	Status          string   `json:"status"`
	Content         string   `json:"content"`
	Summary         string   `json:"summary"`
	SourceRefs      []string `json:"source_refs"`
	ChunkRefs       []string `json:"chunk_refs"`
}

func ValidWikiKBID(id string) bool {
	return id != "" && len(id) <= 128 && id != "." && id != ".." && !strings.ContainsAny(id, "/?#\\%") && strings.IndexFunc(id, unicode.IsControl) < 0
}

// Slugs may contain Unicode and nested folders, but never URL escapes, empty
// path segments or traversal. sendJSON owns URL encoding exactly once.
func ValidWikiSlug(slug string) bool {
	if slug == "" || !utf8.ValidString(slug) || utf8.RuneCountInString(slug) > 255 || strings.TrimSpace(slug) != slug || strings.ContainsAny(slug, "?#\\%") || strings.IndexFunc(slug, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(slug, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func wikiInvalidRequest() error  { return &Error{Code: "upstream.request_invalid", StatusCode: 400} }
func wikiInvalidResponse() error { return &Error{Code: "upstream.invalid_response", StatusCode: 502} }

func (c *Client) WikiEnabled(ctx context.Context, kb string, h http.Header) (bool, error) {
	if !ValidWikiKBID(kb) {
		return false, wikiInvalidRequest()
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ID           string `json:"id"`
			Capabilities struct {
				Wiki *bool `json:"wiki"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := c.getJSONQuery(ctx, "/api/v1/knowledge-bases/"+kb, url.Values{"resource_urls": {"handle"}}, h, &result); err != nil {
		return false, err
	}
	if !result.Success || result.Data.ID != kb || result.Data.Capabilities.Wiki == nil {
		return false, wikiInvalidResponse()
	}
	return *result.Data.Capabilities.Wiki, nil
}

func (c *Client) SearchWikiPages(ctx context.Context, kb, query string, limit int, h http.Header) ([]WikiPage, error) {
	if !ValidWikiKBID(kb) || strings.TrimSpace(query) == "" || utf8.RuneCountInString(query) > 4000 || limit < 1 || limit > 20 {
		return nil, wikiInvalidRequest()
	}
	// The pinned REST implementation uses a PostgreSQL regular expression.
	// MCP accepts literal search text, preventing accidental regex expansion.
	values := url.Values{"q": {regexp.QuoteMeta(query)}, "limit": {strconv.Itoa(limit)}, "resource_urls": {"handle"}}
	var result struct {
		Pages json.RawMessage `json:"pages"`
	}
	if err := c.getJSONQuery(ctx, "/api/v1/knowledgebase/"+kb+"/wiki/search", values, h, &result); err != nil {
		return nil, err
	}
	var pages []WikiPage
	if len(result.Pages) == 0 || json.Unmarshal(result.Pages, &pages) != nil || len(pages) > limit {
		return nil, wikiInvalidResponse()
	}
	for _, page := range pages {
		if !validWikiResponse(page, kb) {
			return nil, wikiInvalidResponse()
		}
	}
	if pages == nil {
		pages = []WikiPage{}
	}
	return pages, nil
}

func (c *Client) GetWikiPage(ctx context.Context, kb, slug string, h http.Header) (WikiPage, error) {
	if !ValidWikiKBID(kb) || !ValidWikiSlug(slug) {
		return WikiPage{}, wikiInvalidRequest()
	}
	var page WikiPage
	if err := c.getJSONQuery(ctx, "/api/v1/knowledgebase/"+kb+"/wiki/pages/"+slug, url.Values{"resource_urls": {"handle"}}, h, &page); err != nil {
		return page, err
	}
	if !validWikiResponse(page, kb) || page.Slug != slug {
		return WikiPage{}, wikiInvalidResponse()
	}
	return page, nil
}

func validWikiResponse(page WikiPage, kb string) bool {
	return page.ID != "" && page.TenantID != 0 && page.KnowledgeBaseID == kb && ValidWikiSlug(page.Slug) && ValidWikiPageType(page.PageType) && (page.Status == "draft" || page.Status == "published" || page.Status == "archived")
}

func ValidWikiPageType(value string) bool {
	switch value {
	case "summary", "entity", "concept", "index", "synthesis", "comparison":
		return true
	}
	return false
}
