package weknora

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnterpriseMembershipPaginationPreservesQueryAndHumanCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tenants/42/members" || r.URL.Query().Get("page") != "2" || r.URL.Query().Get("page_size") != "100" {
			t.Errorf("wrong native URL: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-human" || r.Header.Get("X-Tenant-ID") != "42" || r.Header.Get("X-API-Key") != "" {
			t.Error("original credential or workspace lost")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"members":[],"total":0}}`))
	}))
	defer upstream.Close()
	c := newTestClient(t, upstream.URL, time.Second)
	_, err := c.EnterpriseRequest(context.Background(), "GET", "/api/v1/tenants/42/members?page=2&page_size=100", http.Header{"Authorization": {"Bearer synthetic-human"}, "X-Tenant-Id": {"42"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMachineMemberRequestsPreserveNativeExternalPrincipalProof(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "synthetic-machine" || r.Header.Get("X-Tenant-ID") != "42" || r.Header.Get("X-External-User-ID") != "client-user" || r.Header.Get("X-External-User-Token") != "synthetic-signed-proof" {
			t.Error("native machine identity or proof headers lost")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Untrusted-Forward") != "" {
			t.Error("unexpected caller credentials/headers forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"members":[],"total":0}}`))
	}))
	defer upstream.Close()
	c := newTestClient(t, upstream.URL, time.Second)
	h := make(http.Header)
	h.Set("X-API-Key", "synthetic-machine")
	h.Set("X-Tenant-ID", "42")
	h.Set("X-External-User-ID", "client-user")
	h.Set("X-External-User-Token", "synthetic-signed-proof")
	h.Set("Cookie", "synthetic-session")
	h.Set("X-Untrusted-Forward", "must-not-forward")
	_, err := c.EnterpriseRequest(context.Background(), http.MethodGet, "/api/v1/tenants/42/members?page=1&page_size=1", h, nil)
	if err != nil {
		t.Fatal(err)
	}
}
