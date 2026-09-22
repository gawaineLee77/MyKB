package nativeaccess

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNativeAllModeUsesComputedCapabilities(t *testing.T) {
	s, f, _, actor := testScope()
	f.caps = map[string]map[string]any{"a": {"vector": true}, "b": {"wiki": true}}
	f.agent = map[string]any{"tenant_id": json.Number("1"), "config": map[string]any{"kb_selection_mode": "all", "allowed_tools": []any{"wiki_search"}}}
	ids, err := s.Resolve(context.Background(), nil, false, "agent", actor, http.Header{})
	if err != nil || len(ids) != 1 || ids[0] != "b" {
		t.Fatal(ids, err)
	}
	f.agent["config"].(map[string]any)["agent_mode"] = "quick-answer"
	ids, err = s.Resolve(context.Background(), nil, false, "agent", actor, http.Header{})
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
}

func TestAmbiguousAgentSourceCannotExecuteAnotherConfig(t *testing.T) {
	s, _, store, actor := testScope()
	store.Bind(context.Background(), "session", actor, nil)
	r := requestJSON("POST", "/api/v1/agent-chat/session", `{"query":"synthetic","agent_id":"agent","agent_source_tenant_id":9007199254740993}`)
	if err := s.Check(r.Context(), r, actor); err == nil || err.Error() != "agent.source_ambiguous" {
		t.Fatal(err)
	}
}

func TestNativePayloadPreservesLargeIntegersAndRejectsTrailingJSON(t *testing.T) {
	_, f, _, actor := testScope()
	policy := ModelPolicy{Upstream: f}
	r := requestJSON("POST", "/api/v1/knowledge-bases", `{"future_native_option":{"tenant_id":9007199254740993}}`)
	if err := policy.Check(r.Context(), r, actor); err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r.Body)
	if !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal(string(raw))
	}
	r = requestJSON("POST", "/api/v1/knowledge-bases", `{} {"summary_model_id":"hidden"}`)
	if err := policy.Check(r.Context(), r, actor); err == nil {
		t.Fatal("accepted a second JSON object")
	}
	r = requestJSON("POST", "/api/v1/knowledge-bases", `{"vlm_config":{"base_url":"http://unapproved.invalid","model_name":"raw"}}`)
	if err := policy.Check(r.Context(), r, actor); err == nil {
		t.Fatal("accepted raw VLM provider configuration")
	}
}

func TestOversizedSSELineIsBoundedBeforeEmission(t *testing.T) {
	stream := &checkedStream{source: io.NopCloser(strings.NewReader("")), reader: bufio.NewReader(strings.NewReader("data: " + strings.Repeat("x", 3<<20))), check: func(any) error { return nil }}
	raw, err := io.ReadAll(stream)
	if err == nil || len(raw) != 0 {
		t.Fatal(len(raw), err)
	}
}

type historyFixture struct {
	*nativeFixture
	pages int
}

func (f *historyFixture) NativeRequest(ctx context.Context, method, path string, q url.Values, h http.Header, body any) (json.RawMessage, error) {
	if path != "/api/v1/sessions" {
		return f.nativeFixture.NativeRequest(ctx, method, path, q, h, body)
	}
	f.pages++
	if q.Get("agent_id") != "synthetic" || h.Get("X-API-Key") != "synthetic-key" {
		return nil, &Error{403, "test.lost_filters"}
	}
	rows := []any{map[string]any{"id": "old"}, map[string]any{"id": "mine"}, map[string]any{"id": "other-key"}}
	return json.Marshal(map[string]any{"success": true, "total": 3, "data": rows})
}
func TestNativeHistoryPaginationFiltersLegacyAndOtherCredentials(t *testing.T) {
	s, f, store, actor := testScope()
	native := &historyFixture{nativeFixture: f}
	s.Upstream = native
	store.Bind(context.Background(), "mine", actor, nil)
	store.Bind(context.Background(), "other-key", Actor{Kind: "api_key", ID: "other", TenantID: 1}, nil)
	r := httptest.NewRequest("GET", "/api/v1/sessions?page=1&page_size=1&agent_id=synthetic", nil)
	r.Header.Set("X-API-Key", "synthetic-key")
	w := httptest.NewRecorder()
	handled, err := s.Local(w, r, actor)
	var got struct {
		Total int `json:"total"`
		Data  []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if err != nil || !handled || got.Total != 1 || len(got.Data) != 1 || got.Data[0].ID != "mine" {
		t.Fatal(w.Body.String(), err)
	}
}
