package weknora

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type wikiRoundTripper func(*http.Request) (*http.Response, error)

func (fn wikiRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func wikiTestClient(t *testing.T, response func(*http.Request) string) *Client {
	t.Helper()
	c := newTestClient(t, "http://synthetic-upstream.invalid", time.Second)
	c.httpClient.Transport = wikiRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response(r))), Request: r}, nil
	})
	return c
}

func wikiPageFixture() WikiPage {
	return WikiPage{ID: "page", TenantID: 42, KnowledgeBaseID: "kb", Slug: "设备/冷却 泵", PageType: "entity", Status: "published", Title: "Synthetic", Content: "Synthetic wiki"}
}

func TestWikiAdapterKeepsCredentialAndLiteralScopedContracts(t *testing.T) {
	page := wikiPageFixture()
	encoded, _ := json.Marshal(page)
	calls := 0
	c := wikiTestClient(t, func(r *http.Request) string {
		calls++
		if r.Method != "GET" || r.Header.Get("X-API-Key") != "synthetic-retrieve-key" || r.Header.Get("X-Tenant-ID") != "42" || r.Header.Get("Authorization") != "" || r.URL.Query().Get("resource_urls") != "handle" {
			t.Fatalf("credential or private-resource contract: %s %+v", r.URL, r.Header)
		}
		switch r.URL.Path {
		case "/api/v1/knowledge-bases/kb":
			return `{"success":true,"data":{"id":"kb","capabilities":{"wiki":true}}}`
		case "/api/v1/knowledgebase/kb/wiki/search":
			if r.URL.Query().Get("q") != `泵\[1\]\.\*` || r.URL.Query().Get("limit") != "10" {
				t.Fatal(r.URL)
			}
			return `{"pages":[` + string(encoded) + `]}`
		case "/api/v1/knowledgebase/kb/wiki/pages/设备/冷却 泵":
			if strings.Contains(r.URL.EscapedPath(), "%25") {
				t.Fatal("double encoded slug", r.URL)
			}
			return string(encoded)
		default:
			t.Fatal(r.URL)
			return "{}"
		}
	})
	h := http.Header{"X-Api-Key": {"synthetic-retrieve-key"}, "X-Tenant-Id": {"42"}}
	if enabled, err := c.WikiEnabled(context.Background(), "kb", h); err != nil || !enabled {
		t.Fatal(enabled, err)
	}
	if pages, err := c.SearchWikiPages(context.Background(), "kb", "泵[1].*", 10, h); err != nil || len(pages) != 1 {
		t.Fatal(pages, err)
	}
	if got, err := c.GetWikiPage(context.Background(), "kb", page.Slug, h); err != nil || got.Content != page.Content {
		t.Fatal(got, err)
	}
	if calls != 3 {
		t.Fatal(calls)
	}
}

func TestWikiAdapterRejectsConfusedParentAndInvalidContracts(t *testing.T) {
	for _, response := range []string{
		`{}`, `{"pages":{}}`, `{"pages":[null]}`,
		`{"pages":[{"id":"p","tenant_id":42,"knowledge_base_id":"foreign","slug":"x","status":"published"}]}`,
		`{"pages":[{"id":"p","tenant_id":42,"knowledge_base_id":"kb","slug":"../other","status":"published"}]}`,
	} {
		c := wikiTestClient(t, func(*http.Request) string { return response })
		if _, err := c.SearchWikiPages(context.Background(), "kb", "test", 1, nil); err == nil {
			t.Fatal(response)
		}
	}
	for _, response := range []string{`{"pages":null}`, `{"pages":[]}`} {
		c := wikiTestClient(t, func(*http.Request) string { return response })
		if rows, err := c.SearchWikiPages(context.Background(), "kb", "none", 1, nil); err != nil || rows == nil || len(rows) != 0 {
			t.Fatal(rows, err)
		}
	}
	c := wikiTestClient(t, func(*http.Request) string {
		page := wikiPageFixture()
		page.Slug = "other"
		raw, _ := json.Marshal(page)
		return string(raw)
	})
	if _, err := c.GetWikiPage(context.Background(), "kb", "requested", nil); err == nil {
		t.Fatal("slug substitution accepted")
	}
}

func TestWikiAdapterRejectsUnsafePathsBeforeRequest(t *testing.T) {
	c := wikiTestClient(t, func(*http.Request) string { t.Fatal("invalid path reached transport"); return "{}" })
	for _, slug := range []string{"", "/page", "a//b", "a/../b", "a/./b", "%2e%2e/private", "page?secret", "page#x", "a\\b", "a\x00b", " page "} {
		if _, err := c.GetWikiPage(context.Background(), "kb", slug, nil); err == nil {
			t.Fatal(slug)
		}
	}
	for _, kb := range []string{"../kb", "a%2fb", "", ".", "a?b"} {
		if _, err := c.SearchWikiPages(context.Background(), kb, "query", 10, nil); err == nil {
			t.Fatal(kb)
		}
	}
}

func TestWikiEnabledDistinguishesDisabledFromMissingCapability(t *testing.T) {
	// The pinned KnowledgeBase.MarshalJSON always emits capabilities, and
	// KBCapabilities.Wiki has no omitempty tag, including for disabled Wiki KBs.
	for _, tc := range []struct {
		body             string
		enabled, wantErr bool
	}{
		{`{"success":true,"data":{"id":"kb","capabilities":{"wiki":false}}}`, false, false},
		{`{"success":true,"data":{"id":"kb","capabilities":{"wiki":true}}}`, true, false},
		{`{"success":true,"data":{"id":"kb","capabilities":{}}}`, false, true},
	} {
		c := wikiTestClient(t, func(*http.Request) string { return tc.body })
		enabled, err := c.WikiEnabled(context.Background(), "kb", nil)
		if enabled != tc.enabled || (err != nil) != tc.wantErr {
			t.Fatal(tc.body, enabled, err)
		}
	}
}
