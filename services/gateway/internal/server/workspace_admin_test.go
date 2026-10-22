package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
)

type workspaceAdminCall struct {
	operation, method, suffix string
	principal                 weknora.Principal
	headers                   http.Header
	tenant, keyID             uint64
	body                      json.RawMessage
}

type workspaceAdminRouteStub struct {
	calls   []workspaceAdminCall
	keyUses []struct {
		key    string
		tenant uint64
	}
	err      error
	keyError error
}

var _ WorkspaceAdminService = (*workspaceAdminRouteStub)(nil)

func (s *workspaceAdminRouteStub) record(op string, p weknora.Principal, h http.Header, tenant, key uint64, method, suffix string, body json.RawMessage) (json.RawMessage, error) {
	s.calls = append(s.calls, workspaceAdminCall{op, method, suffix, p, h.Clone(), tenant, key, append(json.RawMessage(nil), body...)})
	return json.RawMessage(`{"success":true,"data":{"id":9,"creator_user_id":"employee"}}`), s.err
}
func (s *workspaceAdminRouteStub) SetDelegation(_ context.Context, p weknora.Principal, h http.Header, tenant uint64, _ workspaceadmin.DelegationInput) (workspaceadmin.DelegationStatus, error) {
	_, err := s.record("delegate", p, h, tenant, 0, "", "", nil)
	return workspaceadmin.DelegationStatus{Configured: true, Authorized: true}, err
}
func (s *workspaceAdminRouteStub) GetDelegationStatus(_ context.Context, p weknora.Principal, h http.Header, tenant uint64) (workspaceadmin.DelegationStatus, error) {
	_, err := s.record("delegation-status", p, h, tenant, 0, "", "", nil)
	return workspaceadmin.DelegationStatus{Configured: true, Authorized: true}, err
}
func (s *workspaceAdminRouteStub) RevokeDelegation(_ context.Context, p weknora.Principal, h http.Header, tenant uint64) error {
	_, err := s.record("revoke-delegation", p, h, tenant, 0, "", "", nil)
	return err
}
func (s *workspaceAdminRouteStub) ListKeys(_ context.Context, p weknora.Principal, h http.Header, tenant uint64) (json.RawMessage, error) {
	return s.record("list-keys", p, h, tenant, 0, "", "", nil)
}
func (s *workspaceAdminRouteStub) CreateKey(_ context.Context, p weknora.Principal, h http.Header, tenant uint64, body json.RawMessage) (json.RawMessage, error) {
	return s.record("create-key", p, h, tenant, 0, "", "", body)
}
func (s *workspaceAdminRouteStub) UpdateKey(_ context.Context, p weknora.Principal, h http.Header, tenant, key uint64, body json.RawMessage) (json.RawMessage, error) {
	return s.record("update-key", p, h, tenant, key, "", "", body)
}
func (s *workspaceAdminRouteStub) RevokeKey(_ context.Context, p weknora.Principal, h http.Header, tenant, key uint64) error {
	_, err := s.record("revoke-key", p, h, tenant, key, "", "", nil)
	return err
}
func (s *workspaceAdminRouteStub) Manage(_ context.Context, p weknora.Principal, h http.Header, tenant uint64, method, suffix string, body json.RawMessage) (json.RawMessage, error) {
	return s.record("manage", p, h, tenant, 0, method, suffix, body)
}
func (s *workspaceAdminRouteStub) ManageMachine(_ context.Context, h http.Header, tenant uint64, method, suffix string, body json.RawMessage) (json.RawMessage, error) {
	return s.record("manage-machine", weknora.Principal{}, h, tenant, 0, method, suffix, body)
}
func (s *workspaceAdminRouteStub) SelfLeave(_ context.Context, p weknora.Principal, h http.Header, tenant uint64) (json.RawMessage, error) {
	return s.record("self-leave", p, h, tenant, 0, "", "", nil)
}
func (s *workspaceAdminRouteStub) DisableCreatorKeys(context.Context, string) error { return s.err }
func (s *workspaceAdminRouteStub) CheckKeyUse(_ context.Context, key string, tenant uint64) error {
	s.keyUses = append(s.keyUses, struct {
		key    string
		tenant uint64
	}{key, tenant})
	return s.keyError
}

func workspaceAdminRouteHandler(t *testing.T, role string, service *workspaceAdminRouteStub, fallback http.Handler) http.Handler {
	t.Helper()
	now := time.Now()
	p := &enterprisePrincipal{p: weknora.Principal{User: &weknora.User{ID: "employee", Email: "alias@example.invalid", IsActive: true}, Tenant: &weknora.Tenant{ID: 42}, Memberships: []weknora.Membership{{TenantID: 42, Role: role}}}}
	store := &enterpriseTestStore{i: enterprise.Installation{Stage: "ready", AdminUserID: "installation-owner"}, o: enterprise.OnboardingRecord{Subject: "employee-subject", CompletedAt: &now}}
	if fallback == nil {
		fallback = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected native fallback: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(598)
		})
	}
	return newNativeHandler(config.Config{}, nil, Dependencies{Principals: p, Enterprise: &enterprise.Service{Store: store, Upstream: p, Identities: enterpriseTestDirectory{}}, WorkspaceAdmin: service, MCP: fallback}, fallback)
}

func workspaceAdminRequest(h http.Handler, method, path, body, bearer, key, tenant string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if bearer != "" {
		r.Header.Set("Authorization", bearer)
	}
	if key != "" {
		r.Header.Set("X-API-Key", key)
	}
	if tenant != "" {
		r.Header.Set("X-Tenant-ID", tenant)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestWorkspaceAdminKeyCRUDUsesProductServiceAndHumanIdentity(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, operation string
		status                        int
		key                           uint64
	}{
		{"GET", "/api/v1/tenants/42/api-keys", "", "list-keys", 200, 0},
		{"POST", "/api/v1/tenants/42/api-keys", `{"name":"synthetic","capabilities":["retrieve"],"knowledge_base_ids":["kb"]}`, "create-key", 201, 0},
		{"PUT", "/api/v1/tenants/42/api-keys/9", `{"name":"renamed","full_access":false}`, "update-key", 200, 9},
		{"DELETE", "/api/v1/tenants/42/api-keys/9", "", "revoke-key", 200, 9},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			s := &workspaceAdminRouteStub{}
			w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "admin", s, nil), tc.method, tc.path, tc.body, "Bearer employee", "", "42")
			if w.Code != tc.status || len(s.calls) != 1 {
				t.Fatalf("status %d calls %d body %s", w.Code, len(s.calls), w.Body.String())
			}
			call := s.calls[0]
			if call.operation != tc.operation || call.tenant != 42 || call.keyID != tc.key || call.principal.User.ID != "employee" || call.principal.Memberships[0].Role != "admin" || call.headers.Get("Authorization") != "Bearer employee" {
				t.Fatalf("wrong authority or route: %+v", call)
			}
			if tc.body != "" && string(call.body) != tc.body {
				t.Fatalf("key payload changed: %s", call.body)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("key material response must not be cached")
			}
			if !json.Valid(w.Body.Bytes()) || !strings.Contains(w.Body.String(), `"success":true`) {
				t.Fatal("native response envelope lost")
			}
		})
	}
}

func TestWorkspaceAdminMemberWritesMapEmailBeforeMediation(t *testing.T) {
	for _, suffix := range []string{"members", "invitations"} {
		for _, role := range []string{"owner", "admin", "contributor", "viewer"} {
			t.Run(suffix+"/"+role, func(t *testing.T) {
				s := &workspaceAdminRouteStub{}
				w := workspaceAdminRequest(workspaceAdminRouteHandler(t, role, s, nil), "POST", "/api/v1/tenants/42/"+suffix, `{"email":"employee@company.invalid","role":"contributor"}`, "Bearer employee", "", "42")
				if role == "contributor" || role == "viewer" {
					if w.Code != 403 || len(s.calls) != 0 {
						t.Fatalf("unprivileged member write reached service: %d %+v", w.Code, s.calls)
					}
					return
				}
				if w.Code != 201 || len(s.calls) != 1 {
					t.Fatalf("status %d body %s", w.Code, w.Body.String())
				}
				call := s.calls[0]
				var input map[string]string
				if json.Unmarshal(call.body, &input) != nil || input["email"] != "alias@example.invalid" || input["role"] != "contributor" || call.suffix != "/"+suffix || call.method != "POST" || call.operation != "manage" {
					t.Fatalf("unmapped or misrouted request: %+v", call)
				}
			})
		}
	}
}

func TestWorkspaceAdminManageAndSelfLeaveAreIntercepted(t *testing.T) {
	for _, tc := range []struct {
		method, suffix, body, operation, role string
		status                                int
	}{
		{"PUT", "", `{"name":"Updated workspace"}`, "manage", "admin", 200},
		{"PUT", "/members/member-two", `{"role":"viewer"}`, "manage", "admin", 200},
		{"DELETE", "/members/member-two", "", "manage", "admin", 200},
		{"DELETE", "/invitations/8", "", "manage", "admin", 200},
		{"POST", "/invite-links", `{"role":"contributor"}`, "manage", "admin", 201},
		{"POST", "/leave", "", "self-leave", "viewer", 200},
	} {
		t.Run(tc.method+tc.suffix, func(t *testing.T) {
			s := &workspaceAdminRouteStub{}
			w := workspaceAdminRequest(workspaceAdminRouteHandler(t, tc.role, s, nil), tc.method, "/api/v1/tenants/42"+tc.suffix, tc.body, "Bearer employee", "", "42")
			if w.Code != tc.status || len(s.calls) != 1 || s.calls[0].operation != tc.operation {
				t.Fatalf("write escaped mediation: %d %+v %s", w.Code, s.calls, w.Body.String())
			}
			if tc.operation == "manage" && (s.calls[0].suffix != tc.suffix || s.calls[0].method != tc.method) {
				t.Fatal("mutation identity not preserved")
			}
		})
	}
}

func TestWorkspaceAdminKeyGuardBlocksRevokedKeysForAPIAndMCP(t *testing.T) {
	for _, path := range []string{"/api/v1/knowledge-bases", "/api/v1/tenants/42/members", "/mcp"} {
		t.Run(path, func(t *testing.T) {
			s := &workspaceAdminRouteStub{keyError: &workspaceadmin.Error{Status: 403, Code: "workspace_admin.key_disabled"}}
			w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, nil), "POST", path, `{}`, "", "synthetic-disabled-key", "42")
			if w.Code != 403 || !strings.Contains(w.Body.String(), "workspace_admin.key_disabled") || len(s.calls) != 0 || len(s.keyUses) != 1 || s.keyUses[0].tenant != 42 || s.keyUses[0].key != "synthetic-disabled-key" {
				t.Fatalf("revoked key passed guard: %d %s %+v", w.Code, w.Body.String(), s)
			}
		})
	}
}

func TestWorkspaceAdminRejectsDualCredentialsAndMachineHumanMasquerade(t *testing.T) {
	for _, path := range []string{"/api/v1/tenants/42/api-keys", "/api/v1/mindcreek/workspace-admin/delegation", "/api/v1/tenants/42/members", "/mcp"} {
		s := &workspaceAdminRouteStub{}
		w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, nil), "POST", path, `{}`, "Bearer employee", "synthetic-key", "42")
		if w.Code != 400 || !strings.Contains(w.Body.String(), "auth.ambiguous_credentials") || len(s.calls) != 0 || len(s.keyUses) != 0 {
			t.Fatalf("dual credentials accepted on %s: %d %+v", path, w.Code, s)
		}
	}
	for _, path := range []string{"/api/v1/tenants/42/api-keys", "/api/v1/mindcreek/workspace-admin/delegation"} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			if method == "DELETE" && strings.HasSuffix(path, "api-keys") {
				continue
			}
			s := &workspaceAdminRouteStub{}
			w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, nil), method, path, `{}`, "", "synthetic-owner-like-key", "42")
			if w.Code != 403 || !strings.Contains(w.Body.String(), "auth.human_required") || len(s.calls) != 0 {
				t.Fatalf("API key impersonated human on %s %s: %d %s", method, path, w.Code, w.Body.String())
			}
		}
	}
}

func TestWorkspaceAdminMachineMembersPreserveCredentialKind(t *testing.T) {
	for _, tc := range []struct{ method, suffix, body string }{
		{"POST", "/members", `{"email":"machine@example.invalid","role":"viewer"}`},
		{"PUT", "/members/u2", `{"role":"viewer"}`},
		{"DELETE", "/members/u2", ""},
		{"POST", "/invitations", `{"email":"machine@example.invalid","role":"viewer"}`},
		{"DELETE", "/invitations/9", ""},
		{"POST", "/invite-links", `{"role":"viewer"}`},
	} {
		t.Run(tc.method+tc.suffix, func(t *testing.T) {
			s := &workspaceAdminRouteStub{}
			w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, nil), tc.method, "/api/v1/tenants/42"+tc.suffix, tc.body, "", "synthetic-machine-member-key", "42")
			if w.Code >= 300 || len(s.calls) != 1 || len(s.keyUses) != 1 {
				t.Fatalf("machine mutation escaped mediation: %d %+v %s", w.Code, s, w.Body.String())
			}
			call := s.calls[0]
			if call.operation != "manage-machine" || call.principal.User != nil || call.headers.Get("Authorization") != "" || call.headers.Get("X-API-Key") != "synthetic-machine-member-key" || call.suffix != tc.suffix || string(call.body) != tc.body {
				t.Fatalf("machine credential was delegated or body altered: %+v", call)
			}
		})
	}
	for _, tc := range []struct{ method, suffix string }{{"PUT", ""}, {"POST", "/leave"}} {
		s := &workspaceAdminRouteStub{}
		forwarded := false
		fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			forwarded = true
			if r.Header.Get("X-API-Key") == "" || r.Header.Get("Authorization") != "" {
				t.Fatal("machine fallback credentials changed")
			}
			w.WriteHeader(204)
		})
		w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, fallback), tc.method, "/api/v1/tenants/42"+tc.suffix, `{}`, "", "synthetic-machine-key", "42")
		if w.Code != 204 || !forwarded || len(s.calls) != 0 || len(s.keyUses) != 1 {
			t.Fatalf("native machine authority changed: %d %+v", w.Code, s)
		}
	}
}

func TestWorkspaceAdminRejectsMalformedIDsAndBodiesBeforeService(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-a-number", "18446744073709551616"} {
		for _, path := range []string{"/api/v1/tenants/" + value + "/api-keys", "/api/v1/tenants/42/api-keys/" + value} {
			s := &workspaceAdminRouteStub{}
			method := "GET"
			if strings.Contains(path, "/api-keys/") {
				method = "DELETE"
			}
			w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "admin", s, nil), method, path, "", "Bearer employee", "", "42")
			if w.Code != 400 || len(s.calls) != 0 {
				t.Fatalf("invalid ID %s reached service: %d %+v", path, w.Code, s.calls)
			}
		}
		s := &workspaceAdminRouteStub{}
		w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "admin", s, nil), "GET", "/api/v1/knowledge-bases", "", "", "synthetic-key", value)
		if w.Code != 403 || len(s.keyUses) != 0 {
			t.Fatalf("invalid machine tenant header reached key service: %q %d", value, w.Code)
		}
	}
	for _, body := range []string{"", "{", `{} {}`, `{"name":"` + strings.Repeat("x", 64<<10) + `"}`} {
		s := &workspaceAdminRouteStub{}
		w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "admin", s, nil), "POST", "/api/v1/tenants/42/api-keys", body, "Bearer employee", "", "42")
		if w.Code != 400 || len(s.calls) != 0 {
			t.Fatalf("invalid body reached key service: %d", w.Code)
		}
	}
}

func TestWorkspaceAdminDelegationTakesNoBrowserCredentialsAndErrorsFailClosed(t *testing.T) {
	for _, body := range []string{`{"refresh_token":"synthetic-browser-token"}`, `{"role":"owner"}`, `[]`, `null`} {
		s := &workspaceAdminRouteStub{}
		w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, nil), "POST", "/api/v1/mindcreek/workspace-admin/delegation", body, "Bearer employee", "", "42")
		if w.Code != 400 || len(s.calls) != 0 {
			t.Fatalf("unexpected delegation input accepted: %s status %d", body, w.Code)
		}
	}
	for _, tc := range []struct{ method, operation string }{{"GET", "delegation-status"}, {"POST", "delegate"}, {"DELETE", "revoke-delegation"}} {
		s := &workspaceAdminRouteStub{}
		w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "owner", s, nil), tc.method, "/api/v1/mindcreek/workspace-admin/delegation", `{}`, "Bearer employee", "", "42")
		if w.Code != 200 || len(s.calls) != 1 || s.calls[0].operation != tc.operation || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("delegation dispatch failed: %d %+v", w.Code, s.calls)
		}
		if strings.Contains(w.Body.String(), "Bearer employee") {
			t.Fatal("session credential leaked")
		}
	}
	s := &workspaceAdminRouteStub{err: &workspaceadmin.Error{Status: 403, Code: "workspace_admin.key_creator_denied"}}
	w := workspaceAdminRequest(workspaceAdminRouteHandler(t, "admin", s, nil), "DELETE", "/api/v1/tenants/42/api-keys/9", "", "Bearer employee", "", "42")
	if w.Code != 403 || !strings.Contains(w.Body.String(), "workspace_admin.key_creator_denied") || strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("service denial masked by success: %d %s", w.Code, w.Body.String())
	}
}
