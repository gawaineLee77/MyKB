package nativeaccess

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateResourceModeOverridesPublicAndDefault(t *testing.T) {
	for _, query := range []string{"", "?resource_urls=public", "?resource_urls=public&resource_urls=handle&other=value"} {
		r := httptest.NewRequest("POST", "/api/v1/knowledge-chat/session"+query, nil)
		if err := enforcePrivateResources(r); err != nil {
			t.Fatal(err)
		}
		if values := r.URL.Query()["resource_urls"]; len(values) != 1 || values[0] != "handle" || !strings.Contains(r.RequestURI, "resource_urls=handle") {
			t.Fatalf("public bypass: %s", r.RequestURI)
		}
	}
}
func TestChatImagesCannotReadUnboundResources(t *testing.T) {
	for _, input := range []string{
		`{"images":[{"url":"resource://other-kb"}]}`,
		`{"images":[{"url":"https://storage.invalid/private"}]}`,
		`{"images":[{"data":"data:image/png;base64,AA==","url":"resource://other"}]}`,
		`{"images":[{"data":"data:image/png;base64,AA==","caption":"forged"}]}`,
	} {
		var value map[string]any
		_ = json.Unmarshal([]byte(input), &value)
		if checkResourceInputs(value) == nil {
			t.Fatalf("unsafe image accepted %s", input)
		}
	}
	var value map[string]any
	_ = json.Unmarshal([]byte(`{"images":[{"data":"data:image/png;base64,AA=="}],"attachment_ids":["session-bound"],"attachment_uploads":[{"data":"AA==","file_name":"synthetic.txt"}]}`), &value)
	if err := checkResourceInputs(value); err != nil {
		t.Fatal(err)
	}
}
func TestACLStreamValidatesBeforeReencodingResourceFields(t *testing.T) {
	input := "event: references\nid: 17\ndata: {\ndata: \"data\":{\"file_path\":\"s3://private\",\"content\":\"allowed text\",\"image\":\"resource://protected\",\"url\":\"https://store.invalid/a?X-Amz-Signature=secret\"}}\n\n"
	checked := false
	source := io.NopCloser(strings.NewReader(input))
	stream := &checkedStream{source: source, reader: bufio.NewReader(source), scrub: true, check: func(value any) error { checked = true; return nil }}
	result, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	output := string(result)
	if !checked || !strings.Contains(output, "event: references\nid: 17\n") || !strings.Contains(output, "allowed text") || !strings.Contains(output, "resource://protected") {
		t.Fatalf("lost event metadata/content: %s", output)
	}
	if strings.Contains(output, "file_path") || strings.Contains(output, "Signature") || strings.Contains(output, "secret") {
		t.Fatalf("storage leak: %s", output)
	}
}
func TestACLHistoryScrubsStorageMetadata(t *testing.T) {
	s := &ScopeService{KnowledgeAccess: func(context.Context, string, http.Header) error { return nil }}
	r := httptest.NewRequest("GET", "/api/v1/messages/session", nil)
	r = r.WithContext(context.WithValue(r.Context(), scopeContextKey{}, &requestState{}))
	response := &http.Response{StatusCode: 200, Request: r, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":[{"content":"safe","images":[{"url":"resource://safe","file_url":"https://storage.invalid/a"}],"file_path":"local://secret"}]}`))}
	if err := s.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	if strings.Contains(string(raw), "file_") || strings.Contains(string(raw), "storage.invalid") || !strings.Contains(string(raw), "resource://safe") {
		t.Fatalf("history leak %s", raw)
	}
	if response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal("history cache enabled")
	}
}
func TestACLWorkspaceMetadataDoesNotDiscloseHiddenCounts(t *testing.T) {
	s := &ScopeService{KnowledgeAccess: func(context.Context, string, http.Header) error { return nil }}
	r := httptest.NewRequest("GET", "/api/v1/organizations", nil)
	response := &http.Response{StatusCode: 200, Request: r, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"space","name":"Synthetic","resource_counts":{"knowledge_base":9},"knowledge_count":99,"agent_count":4}]}`))}
	if err := s.FilterWorkspaceMetadata(response); err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	if strings.Contains(string(raw), "count") || !strings.Contains(string(raw), "Synthetic") {
		t.Fatalf("aggregate leak %s", raw)
	}
}
