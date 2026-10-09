package graphview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type fixture struct {
	role                    string
	tenant, kbTenant        uint64
	active, enabled, denyKB bool
	reads                   int
	query                   Query
}

func (f *fixture) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return weknora.Principal{User: &weknora.User{ID: "human", IsActive: f.active, IsSystemAdmin: true}, Tenant: &weknora.Tenant{ID: f.tenant}, Memberships: []weknora.Membership{{TenantID: f.tenant, Role: f.role}}}, nil
}
func (f *fixture) NativeRequest(_ context.Context, _ string, _ string, _ url.Values, h http.Header, _ any) (json.RawMessage, error) {
	if h.Get("Authorization") != "Bearer human" {
		return nil, fmt.Errorf("original credential missing")
	}
	if f.denyKB {
		return nil, &weknora.Error{StatusCode: 403}
	}
	return json.RawMessage(fmt.Sprintf(`{"success":true,"data":{"id":"kb-a","tenant_id":%d,"indexing_strategy":{"graph_enabled":%t}}}`, f.kbTenant, f.enabled)), nil
}
func (f *fixture) KnowledgeBaseForKnowledge(_ context.Context, id string, _ http.Header) (string, error) {
	if id == "deleted" {
		return "", &weknora.Error{StatusCode: 404}
	}
	if id == "foreign" {
		return "kb-other", nil
	}
	return "kb-a", nil
}
func (f *fixture) Read(_ context.Context, q Query) (Graph, error) {
	f.reads++
	f.query = q
	return Graph{Nodes: []Node{{ID: "1", Name: "A", KnowledgeID: "doc"}, {ID: "2", Name: "B", KnowledgeID: "deleted"}, {ID: "3", Name: "C", KnowledgeID: "foreign"}}, Edges: []Edge{{"e1", "1", "2", "USES"}, {"e2", "1", "3", "USES"}}}, nil
}
func newFixture() *fixture {
	return &fixture{role: "owner", tenant: 1, kbTenant: 1, active: true, enabled: true}
}
func request(s *Service, path string, headers http.Header) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle(Route, s)
	r := httptest.NewRequest("GET", path, nil)
	r.Header = headers
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

const endpoint = "/api/v1/mindcreek/graph/knowledge-bases/kb-a"

func TestPermissionsBeforeDatabase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fixture, http.Header)
		status int
	}{
		{"owner", func(f *fixture, h http.Header) {}, 200},
		{"admin", func(f *fixture, h http.Header) { f.role = "admin" }, 200},
		{"contributor", func(f *fixture, h http.Header) { f.role = "contributor" }, 200},
		{"viewer even platform admin", func(f *fixture, h http.Header) { f.role = "viewer" }, 403},
		{"removed", func(f *fixture, h http.Header) { f.role = "" }, 403},
		{"inactive", func(f *fixture, h http.Header) { f.active = false }, 403},
		{"unauthenticated", func(f *fixture, h http.Header) { h.Del("Authorization") }, 401},
		{"machine owner", func(f *fixture, h http.Header) { h.Set("X-API-Key", "synthetic") }, 403},
		{"tenant spoof", func(f *fixture, h http.Header) { h.Set("X-Tenant-ID", "2") }, 403},
		{"foreign KB", func(f *fixture, h http.Header) { f.kbTenant = 2 }, 403},
		{"native denied or shared revoked", func(f *fixture, h http.Header) { f.denyKB = true }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			h := http.Header{"Authorization": []string{"Bearer human"}}
			tc.change(f, h)
			s := &Service{Enabled: true, Upstream: f, Reader: f}
			w := request(s, endpoint, h)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if tc.status != 200 && f.reads != 0 {
				t.Fatal("denial reached graph database")
			}
		})
	}
}
func TestScopedQuerySourceCleanupAndValidation(t *testing.T) {
	f := newFixture()
	s := &Service{Enabled: true, Upstream: f, Reader: f}
	h := http.Header{"Authorization": []string{"Bearer human"}}
	w := request(s, endpoint+"?q=A&relation=USES&node_id=node-1&limit=60", h)
	var response struct {
		Data Graph `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	if w.Code != 200 || len(response.Data.Nodes) != 1 || len(response.Data.Edges) != 0 || f.query.KBID != "kb-a" || f.query.NodeID != "node-1" || f.query.Relation != "USES" {
		t.Fatalf("unsafe result: %s", w.Body.String())
	}
	for _, suffix := range []string{"?limit=0", "?limit=161", "?limit=abc", "?cypher=MATCH", "?q=a&q=b"} {
		before := f.reads
		w = request(s, endpoint+suffix, h)
		if w.Code != 400 || before != f.reads {
			t.Fatalf("invalid query reached graph: %s", suffix)
		}
	}
	f.enabled = false
	before := f.reads
	w = request(s, endpoint, h)
	if w.Code != 200 || before != f.reads {
		t.Fatal("disabled KB was queried")
	}
	s.Enabled = false
	w = request(s, endpoint, h)
	if w.Code != 409 || before != f.reads {
		t.Fatal("disabled deployment was queried")
	}
}
func TestPermissionRecheckedOnExpansion(t *testing.T) {
	f := newFixture()
	s := &Service{Enabled: true, Upstream: f, Reader: f}
	h := http.Header{"Authorization": []string{"Bearer human"}}
	if request(s, endpoint, h).Code != 200 {
		t.Fatal("initial failed")
	}
	f.role = "viewer"
	if request(s, endpoint+"?node_id=1", h).Code != 403 || f.reads != 1 {
		t.Fatal("revoked expansion queried graph")
	}
}
