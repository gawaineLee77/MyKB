package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/branding"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type brandStore struct{ record branding.Record }

func (s *brandStore) Get(context.Context) (branding.Record, error) { return s.record, nil }
func (s *brandStore) Update(_ context.Context, c branding.Change, _, _ string) (branding.Record, error) {
	if s.record.Revision != c.Revision {
		return branding.Record{}, branding.ErrConflict
	}
	s.record.Name = c.Name
	s.record.Revision++
	return s.record, nil
}

func TestBrandingHTTPIdentityBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, id, credential, body string
		platform                                 bool
		want                                     int
	}{
		{"anonymous branding", "GET", branding.Route, "", "", "", false, 200},
		{"anonymous logo", "GET", branding.Route + "/logo", "", "", "", false, 200},
		{"anonymous write", "PUT", branding.Route, "", "", `{"name":"changed","revision":1}`, false, 403},
		{"key cannot impersonate", "PUT", branding.Route, "installed", "key", `{"name":"changed","revision":1}`, true, 403},
		{"space owner denied", "PUT", branding.Route, "employee", "human", `{"name":"changed","revision":1}`, false, 403},
		{"other platform admin denied", "PUT", branding.Route, "employee", "human", `{"name":"changed","revision":1}`, true, 403},
		{"installed admin", "PUT", branding.Route, "installed", "human", `{"name":"changed","revision":1}`, true, 200},
		{"unknown field", "PUT", branding.Route, "installed", "human", `{"name":"changed","revision":1,"admin":true}`, true, 400},
		{"trailing object", "PUT", branding.Route, "installed", "human", `{"name":"changed","revision":1}{}`, true, 400},
		{"conflict", "PUT", branding.Route, "installed", "human", `{"name":"changed","revision":2}`, true, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			installation := &enterpriseTestStore{i: enterprise.Installation{Stage: "ready", AdminUserID: "installed"}, o: enterprise.OnboardingRecord{Subject: "employee-subject", CompletedAt: &now}}
			p := &enterprisePrincipal{p: weknora.Principal{User: &weknora.User{ID: tc.id, Email: "alias@example.invalid", IsActive: true, IsSystemAdmin: tc.platform}}}
			d := Dependencies{Enterprise: &enterprise.Service{Store: installation, Upstream: p, Identities: enterpriseTestDirectory{}}, Principals: p, Branding: &branding.Service{Store: &brandStore{record: branding.Record{Name: "小纳", Revision: 1, Logo: []byte("synthetic PNG")}}, Installations: installation}}
			mux := http.NewServeMux()
			registerBrandingRoutes(mux, d)
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.credential == "human" {
				r.Header.Set("Authorization", "Bearer synthetic")
				r.Header.Set("X-Tenant-ID", "stale-workspace")
			}
			if tc.credential == "key" {
				r.Header.Set("X-API-Key", "synthetic")
			}
			w := httptest.NewRecorder()
			enterpriseIdentityMiddleware(mux, d).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want == 200 && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable branding")
			}
			if tc.method == "GET" && strings.Contains(w.Body.String(), "installed") {
				t.Fatal("admin identity leaked")
			}
		})
	}
}
