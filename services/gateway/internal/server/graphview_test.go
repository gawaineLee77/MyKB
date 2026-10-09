package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/graphview"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type graphRouteFixture struct {
	enterprisePrincipal
	reads int
}

func (f *graphRouteFixture) NativeRequest(context.Context, string, string, url.Values, http.Header, any) (json.RawMessage, error) {
	return json.RawMessage(`{"success":true,"data":{"id":"kb-a","tenant_id":42,"indexing_strategy":{"graph_enabled":true}}}`), nil
}
func (f *graphRouteFixture) KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error) {
	return "kb-a", nil
}
func (f *graphRouteFixture) Read(context.Context, graphview.Query) (graphview.Graph, error) {
	f.reads++
	return graphview.Graph{Nodes: []graphview.Node{}, Edges: []graphview.Edge{}}, nil
}

type graphIdentity struct {
	enterpriseTestDirectory
	suspended bool
}

func (f graphIdentity) GetByUpstreamEmail(ctx context.Context, email string) (identity.Identity, error) {
	i, e := f.enterpriseTestDirectory.GetByUpstreamEmail(ctx, email)
	if f.suspended {
		i.Status = identity.StatusSuspended
	}
	return i, e
}
func TestGraphRouteThroughEnterpriseAndRetirementGuards(t *testing.T) {
	for _, tc := range []struct {
		name, id, role, path, method string
		completed, suspended         bool
		status                       int
	}{
		{"admin owner", "admin", "owner", "/api/v1/mindcreek/graph/knowledge-bases/kb-a", "GET", true, false, 200},
		{"employee contributor", "employee", "contributor", "/api/v1/mindcreek/graph/knowledge-bases/kb-a", "GET", true, false, 200},
		{"viewer denied", "employee", "viewer", "/api/v1/mindcreek/graph/knowledge-bases/kb-a", "GET", true, false, 403},
		{"onboarding required", "employee", "contributor", "/api/v1/mindcreek/graph/knowledge-bases/kb-a", "GET", false, false, 409},
		{"suspended employee", "employee", "contributor", "/api/v1/mindcreek/graph/knowledge-bases/kb-a", "GET", true, true, 403},
		{"retired namespace remains closed", "admin", "owner", "/api/v1/mindcreek/knowledge-bases/kb-a/grants", "GET", true, false, 410},
		{"no write route", "admin", "owner", "/api/v1/mindcreek/graph/knowledge-bases/kb-a", "POST", true, false, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &graphRouteFixture{enterprisePrincipal: enterprisePrincipal{p: weknora.Principal{User: &weknora.User{ID: tc.id, Email: "alias@example.invalid", IsActive: true, IsSystemAdmin: tc.id == "admin"}, Tenant: &weknora.Tenant{ID: 42}, Memberships: []weknora.Membership{{TenantID: 42, Role: tc.role}}}}}
			store := &enterpriseTestStore{i: enterprise.Installation{Stage: "ready", AdminUserID: "admin", DefaultTenantID: 42}}
			if tc.completed {
				now := time.Now()
				store.o = enterprise.OnboardingRecord{Subject: "employee-subject", CompletedAt: &now}
			}
			d := Dependencies{Principals: f, Enterprise: &enterprise.Service{Store: store, Upstream: f, Identities: graphIdentity{suspended: tc.suspended}}, GraphView: &graphview.Service{Enabled: true, Upstream: f, Reader: f}}
			h := newNativeHandler(config.Config{}, nil, d, http.NotFoundHandler())
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer synthetic")
			r.Header.Set("X-Tenant-ID", "42")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if (f.reads > 0) != (tc.status == 200) {
				t.Fatalf("unexpected database calls: %d", f.reads)
			}
		})
	}
}
