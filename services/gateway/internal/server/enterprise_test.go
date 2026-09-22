package server

import (
	"context"
	"encoding/json"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type enterpriseTestStore struct {
	i enterprise.Installation
	o enterprise.OnboardingRecord
}

func (s *enterpriseTestStore) Installation(context.Context) (enterprise.Installation, error) {
	return s.i, nil
}
func (s *enterpriseTestStore) SaveInstallation(_ context.Context, i enterprise.Installation) error {
	s.i = i
	return nil
}
func (s *enterpriseTestStore) WithLock(_ context.Context, _ string, f func(enterprise.Store) error) error {
	return f(s)
}
func (s *enterpriseTestStore) Onboarding(context.Context, string) (enterprise.OnboardingRecord, error) {
	if s.o.Subject == "" {
		return s.o, enterprise.ErrNotFound
	}
	return s.o, nil
}
func (s *enterpriseTestStore) SaveOnboarding(_ context.Context, o enterprise.OnboardingRecord) error {
	s.o = o
	return nil
}

type enterpriseTestDirectory struct{}

func (enterpriseTestDirectory) GetByUpstreamEmail(context.Context, string) (identity.Identity, error) {
	return identity.Identity{BrokerSubject: "employee-subject", UpstreamEmail: "alias@example.invalid", Status: identity.StatusActive, LocalUserID: "employee"}, nil
}
func (enterpriseTestDirectory) BindLocalAccount(context.Context, string, string) error { return nil }
func (enterpriseTestDirectory) MemberAlias(context.Context, string) (string, error) {
	return "alias@example.invalid", nil
}

type enterprisePrincipal struct{ p weknora.Principal }

func (e *enterprisePrincipal) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return e.p, nil
}
func (e *enterprisePrincipal) EnterpriseRequest(context.Context, string, string, http.Header, any) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"success": true, "data": map[string]any{"user": e.p.User, "tenant": e.p.Tenant, "memberships": e.p.Memberships, "capabilities": map[string]bool{"can_create_tenant": true}}})
}

func TestEnterpriseTenantlessAndProvisioningRouteBoundaries(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name, path, method, id, stage string
		tenant                        uint64
		completed, platform           bool
		want                          int
	}{
		{"admin status", "/api/v1/mindcreek/installation", "GET", "admin", "account_ready", 0, false, true, 200},
		{"admin account", "/api/v1/auth/me", "GET", "admin", "account_ready", 0, false, true, 200},
		{"admin not ready business", "/api/v1/knowledge-bases", "GET", "admin", "account_ready", 0, false, true, 409},
		{"employee account", "/api/v1/auth/me", "GET", "employee", "ready", 0, false, false, 200},
		{"employee pre-onboarding business", "/api/v1/knowledge-bases", "GET", "employee", "ready", 42, false, false, 409},
		{"removed business", "/api/v1/knowledge-bases", "GET", "employee", "ready", 0, true, false, 409},
		{"owner cannot create", "/api/v1/tenants", "POST", "employee", "ready", 42, true, false, 403},
		{"admin creates without tenant", "/api/v1/tenants", "POST", "admin", "ready", 0, false, true, 204},
		{"employee cannot installation", "/api/v1/mindcreek/installation", "GET", "employee", "ready", 42, true, false, 403},
		{"password stays closed", "/api/v1/auth/login", "POST", "employee", "ready", 42, true, false, 404},
		{"employee refresh stays closed", "/api/v1/auth/refresh", "POST", "employee", "ready", 42, true, false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &enterpriseTestStore{i: enterprise.Installation{Stage: tc.stage, AdminUserID: "admin", DefaultTenantID: 42}}
			if tc.completed {
				store.o = enterprise.OnboardingRecord{Subject: "employee-subject", CompletedAt: &now}
			}
			principal := &enterprisePrincipal{p: weknora.Principal{User: &weknora.User{ID: tc.id, Email: "alias@example.invalid", IsActive: true, IsSystemAdmin: tc.platform}}}
			if tc.tenant != 0 {
				principal.p.Tenant = &weknora.Tenant{ID: tc.tenant}
				principal.p.Memberships = []weknora.Membership{{TenantID: tc.tenant, Role: "owner"}}
			}
			deps := Dependencies{Principals: principal, Enterprise: &enterprise.Service{Store: store, Upstream: principal, Identities: enterpriseTestDirectory{}}}
			mux := http.NewServeMux()
			registerEnterpriseRoutes(mux, deps)
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer synthetic")
			w := httptest.NewRecorder()
			enterpriseIdentityMiddleware(mux, deps).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
			if tc.name == "employee account" && strings.Contains(w.Body.String(), `"can_create_tenant":true`) {
				t.Fatal("employee creation capability exposed")
			}
		})
	}
}

func TestEnterpriseMemberOwnerAndEmailMapping(t *testing.T) {
	for _, role := range []string{"owner", "admin", "contributor", "viewer"} {
		t.Run(role, func(t *testing.T) {
			now := time.Now()
			p := &enterprisePrincipal{p: weknora.Principal{User: &weknora.User{ID: "employee", IsActive: true}, Tenant: &weknora.Tenant{ID: 42}, Memberships: []weknora.Membership{{TenantID: 42, Role: role}}}}
			store := &enterpriseTestStore{i: enterprise.Installation{Stage: "ready", AdminUserID: "admin"}, o: enterprise.OnboardingRecord{Subject: "employee-subject", CompletedAt: &now}}
			deps := Dependencies{Principals: p, Enterprise: &enterprise.Service{Store: store, Upstream: p, Identities: enterpriseTestDirectory{}}}
			handler := enterpriseIdentityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(raw), "alias@example.invalid") || r.Header.Get("Authorization") != "Bearer employee" {
					t.Error("member authority or mapping changed")
				}
				w.WriteHeader(201)
			}), deps)
			req := httptest.NewRequest("POST", "/api/v1/tenants/42/invitations", strings.NewReader(`{"email":"employee@company.invalid","role":"contributor"}`))
			req.Header.Set("Authorization", "Bearer employee")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			want := 403
			if role == "owner" {
				want = 201
			}
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
		})
	}
}

func TestEnterpriseSettingsAndMachineCreationCannotBypass(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, key string
		want                    int
	}{
		{"PUT", "/api/v1/system/admin/settings/auth.default_tenant_mode", `{"value":"create_personal"}`, "", 409},
		{"DELETE", "/api/v1/system/admin/settings/auth.registration_mode", `{}`, "", 409},
		{"POST", "/api/v1/tenants", `{"name":"forbidden"}`, "scoped-key", 403},
	} {
		t.Run(tc.path+tc.method, func(t *testing.T) {
			d := Dependencies{Enterprise: &enterprise.Service{Store: &enterpriseTestStore{i: enterprise.Installation{Stage: "ready"}}}}
			handler := enterpriseIdentityMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unsafe request reached upstream") }), d)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.key != "" {
				req.Header.Set("X-API-Key", tc.key)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d", w.Code)
			}
		})
	}
}

func TestEnterpriseRejectsMixedCredentialsBeforeResolvingIdentity(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("mixed credentials forwarded") })
	r := httptest.NewRequest("GET", "/api/v1/knowledge-bases", nil)
	r.Header.Set("Authorization", "Bearer employee")
	r.Header.Set("X-API-Key", "owner-key")
	w := httptest.NewRecorder()
	enterpriseIdentityMiddleware(next, Dependencies{}).ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestEnterpriseAdminAuthenticationRateLimit(t *testing.T) {
	d := Dependencies{Enterprise: &enterprise.Service{Store: &enterpriseTestStore{i: enterprise.Installation{AdminUserID: "admin", AdminEmail: "admin@example.invalid"}}}}
	mux := http.NewServeMux()
	registerEnterpriseRoutes(mux, d)
	for n := 0; n < 21; n++ {
		r := httptest.NewRequest("POST", "/api/v1/mindcreek/admin/auth/login", strings.NewReader(`{"email":"wrong@example.invalid","password":"invalid"}`))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		want := 401
		if n == 20 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: %d", n, w.Code)
		}
	}
}
