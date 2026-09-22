package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

func writeEnterpriseError(w http.ResponseWriter, r *http.Request, err error) {
	var e *enterprise.Error
	if errors.As(err, &e) {
		apierror.Write(w, e.Status, e.Code, "Enterprise operation could not be completed", requestID(r))
		return
	}
	var upstream *weknora.Error
	if errors.As(err, &upstream) {
		apierror.Write(w, upstream.StatusCode, upstream.Code, "Upstream operation could not be completed", requestID(r))
		return
	}
	apierror.Write(w, 503, enterprise.PublicError(err), "Enterprise service is unavailable", requestID(r))
}

type authLimit struct {
	count int
	until time.Time
}

func registerEnterpriseRoutes(mux *http.ServeMux, d Dependencies) {
	if d.Enterprise == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/mindcreek/auth/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"enterprise_enabled": true, "assistant_enabled": d.Assistant != nil && d.Assistant.Enabled})
	})
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" /api/v1/mindcreek/onboarding", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" || r.Header.Get("X-API-Key") != "" {
				apierror.Write(w, 401, "auth.required", "Employee session required", requestID(r))
				return
			}
			if method == "POST" {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
				if err != nil || (len(bytes.TrimSpace(body)) != 0 && string(bytes.TrimSpace(body)) != "{}") {
					apierror.Write(w, 400, "onboarding.input_invalid", "Onboarding takes no workspace or role input", requestID(r))
					return
				}
			}
			status, err := d.Enterprise.Onboarding(r.Context(), r.Header, method == "POST")
			if err != nil {
				writeEnterpriseError(w, r, err)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, map[string]any{"success": true, "data": status})
		})
	}
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalFromRequest(r)
		if !ok {
			apierror.Write(w, 401, "auth.required", "Human session required", requestID(r))
			return
		}
		h := r.Header.Clone()
		if h.Get("X-API-Key") != "" {
			apierror.Write(w, 400, "auth.ambiguous", "Use one authentication credential", requestID(r))
			return
		}
		h.Del("X-API-Key")
		if p.Tenant == nil {
			h.Del("X-Tenant-ID")
		} else {
			h.Set("X-Tenant-ID", strconv.FormatUint(p.Tenant.ID, 10))
		}
		raw, err := d.Enterprise.Upstream.EnterpriseRequest(r.Context(), "GET", "/api/v1/auth/me", h, nil)
		if err != nil {
			writeEnterpriseError(w, r, err)
			return
		}
		var result map[string]any
		if json.Unmarshal(raw, &result) != nil {
			apierror.Write(w, 502, "upstream.invalid_response", "Invalid identity response", requestID(r))
			return
		}
		data, ok := result["data"].(map[string]any)
		if !ok {
			apierror.Write(w, 502, "upstream.invalid_response", "Invalid identity response", requestID(r))
			return
		}
		caps, _ := data["capabilities"].(map[string]any)
		if caps == nil {
			caps = map[string]any{}
		}
		caps["can_create_tenant"] = p.User != nil && p.User.IsActive && p.User.IsSystemAdmin
		data["capabilities"] = caps
		i, err := d.Enterprise.Store.Installation(r.Context())
		if err != nil {
			writeEnterpriseError(w, r, err)
			return
		}
		data["mindcreek"] = map[string]any{"enterprise_enabled": true, "local_admin": i.IsAdmin(p), "installation_ready": i.Stage == "ready"}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, result)
	})
	var mu sync.Mutex
	attempts := map[string]authLimit{}
	for _, action := range []string{"login", "refresh", "logout", "change-password"} {
		mux.HandleFunc("POST /api/v1/mindcreek/admin/auth/"+action, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if action == "login" || action == "refresh" {
				remote, _, _ := net.SplitHostPort(r.RemoteAddr)
				now := time.Now()
				mu.Lock()
				for key, item := range attempts {
					if !now.Before(item.until) {
						delete(attempts, key)
					}
				}
				item, exists := attempts[remote]
				if item.until.IsZero() {
					item.until = now.Add(time.Minute)
				}
				denied := item.count >= 20 || (!exists && len(attempts) >= 1024)
				if !denied {
					item.count++
					attempts[remote] = item
				}
				mu.Unlock()
				if denied {
					w.Header().Set("Retry-After", "60")
					apierror.Write(w, 429, "admin.rate_limited", "Retry later", requestID(r))
					return
				}
			}
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
			if err != nil || (len(raw) > 0 && !json.Valid(raw)) {
				apierror.Write(w, 400, "request.invalid", "Invalid authentication request", requestID(r))
				return
			}
			if len(raw) == 0 {
				raw = []byte(`{}`)
			}
			result, err := d.Enterprise.AdminAuth(r.Context(), action, r.Header, raw)
			if err != nil {
				writeEnterpriseError(w, r, err)
				return
			}
			writeJSON(w, 200, result)
		})
	}
	mux.HandleFunc("GET /api/v1/mindcreek/installation", func(w http.ResponseWriter, r *http.Request) {
		i, err := d.Enterprise.Store.Installation(r.Context())
		if err != nil {
			writeEnterpriseError(w, r, err)
			return
		}
		p, ok := principalFromRequest(r)
		if !ok || !i.IsAdmin(p) {
			apierror.Write(w, 403, "admin.required", "Installation administrator required", requestID(r))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"success": true, "data": i})
	})
}

func enterpriseIdentityMiddleware(next http.Handler, d Dependencies) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Header.Get("Authorization") != "" && r.Header.Get("X-API-Key") != "" {
			apierror.Write(w, 400, "auth.ambiguous_credentials", "Use one authentication credential", requestID(r))
			return
		}
		if (!strings.HasPrefix(path, "/api/v1/") && path != "/mcp") || path == "/api/v1/mindcreek/auth/config" || strings.HasPrefix(path, "/api/v1/mindcreek/admin/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		if closedEnterprisePasswordRoute(r.Method, path) {
			apierror.Write(w, 404, "identity.closed_registration", "Local employee passwords and public registration are disabled", requestID(r))
			return
		}
		if methodPath(r) == "POST /api/v1/auth/refresh" {
			apierror.Write(w, 401, "identity.reauthentication_required", "Corporate sign-in must be renewed", requestID(r))
			return
		}
		if !protectEnterpriseSettings(w, r) {
			return
		}
		if path == "/api/v1/auth/oidc/config" {
			next.ServeHTTP(w, r)
			return
		}
		i, err := d.Enterprise.Store.Installation(r.Context())
		if err != nil {
			writeEnterpriseError(w, r, err)
			return
		}
		if strings.HasPrefix(path, "/api/v1/auth/oidc/") || strings.HasPrefix(path, "/api/v1/mindcreek/oidc/") {
			if i.Stage != "ready" {
				apierror.Write(w, 503, "install.not_ready", "Installation is not ready", requestID(r))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		creating := methodPath(r) == "POST /api/v1/tenants"
		if creating && (r.Header.Get("Authorization") == "" || r.Header.Get("X-API-Key") != "") {
			apierror.Write(w, 403, "workspace.platform_admin_required", "A platform administrator session is required", requestID(r))
			return
		}
		if r.Header.Get("Authorization") == "" {
			if i.Stage != "ready" {
				apierror.Write(w, 503, "install.not_ready", "Installation is not ready", requestID(r))
				return
			}
			if path == "/api/v1/auth/invitations/lookup" {
				apierror.Write(w, 401, "auth.required", "Corporate session required", requestID(r))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if d.Principals == nil {
			apierror.Write(w, 503, "identity.unavailable", "Identity service unavailable", requestID(r))
			return
		}
		h := r.Header.Clone()
		h.Del("X-API-Key")
		if identityRecoveryRoute(r) && path != "/api/v1/auth/me" {
			h.Del("X-Tenant-ID")
		}
		p, err := d.Principals.CurrentPrincipal(r.Context(), h)
		if err != nil && path == "/api/v1/auth/me" && h.Get("X-Tenant-ID") != "" {
			h.Del("X-Tenant-ID")
			p, err = d.Principals.CurrentPrincipal(r.Context(), h)
		}
		if err != nil {
			writePrincipalError(w, r, err)
			return
		}
		if i.IsAdmin(p) {
			if i.Stage != "ready" && path != "/api/v1/auth/me" && path != "/api/v1/mindcreek/installation" && path != "/api/v1/auth/logout" {
				apierror.Write(w, 409, "install.not_ready", "Installation recovery is required", requestID(r))
				return
			}
		} else {
			if i.Stage != "ready" {
				apierror.Write(w, 503, "install.not_ready", "Installation is not ready", requestID(r))
				return
			}
			id, err := d.Enterprise.CheckEmployee(r.Context(), p)
			if err != nil {
				writeEnterpriseError(w, r, err)
				return
			}
			if !identityRecoveryRoute(r) {
				o, err := d.Enterprise.Store.Onboarding(r.Context(), id.BrokerSubject)
				if err != nil && !errors.Is(err, enterprise.ErrNotFound) {
					writeEnterpriseError(w, r, err)
					return
				}
				if o.CompletedAt == nil {
					apierror.Write(w, 409, "onboarding.required", "Complete employee onboarding first", requestID(r))
					return
				}
			}
		}
		if creating && (p.User == nil || !p.User.IsActive || !p.User.IsSystemAdmin) {
			apierror.Write(w, 403, "workspace.platform_admin_required", "A platform administrator session is required", requestID(r))
			return
		}
		if !identityRecoveryRoute(r) && !creating && (p.Tenant == nil || p.Tenant.ID == 0) {
			apierror.Write(w, 409, "workspace.required", "No accessible workspace", requestID(r))
			return
		}
		if !mapEnterpriseMemberEmail(w, r, d, p) {
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func closedEnterprisePasswordRoute(method, path string) bool {
	if method != "POST" {
		return false
	}
	switch path {
	case "/api/v1/auth/register", "/api/v1/auth/register-by-invite", "/api/v1/auth/login", "/api/v1/auth/auto-setup", "/api/v1/auth/change-password":
		return true
	}
	return false
}
func identityRecoveryRoute(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/auth/me", "/api/v1/auth/me/preferences", "/api/v1/auth/logout", "/api/v1/auth/validate", "/api/v1/auth/switch-tenant", "/api/v1/auth/invitations/lookup", "/api/v1/mindcreek/onboarding", "/api/v1/mindcreek/installation":
		return true
	}
	return r.URL.Path == "/api/v1/me/invitations" || strings.HasPrefix(r.URL.Path, "/api/v1/me/invitations/")
}

func protectEnterpriseSettings(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/system/admin/settings/") || r.Method == "GET" {
		return true
	}
	key := strings.TrimPrefix(r.URL.Path, "/api/v1/system/admin/settings/")
	expected, protected := map[string]any{"auth.default_tenant_mode": "tenantless", "auth.registration_mode": "invite_only", "tenant.self_service_creation_enabled": true}[key]
	if !protected {
		return true
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	var body struct {
		Value any `json:"value"`
	}
	if r.Method != "PUT" || err != nil || json.Unmarshal(raw, &body) != nil || body.Value != expected {
		apierror.Write(w, 409, "enterprise.setting_protected", "This setting is managed by enterprise installation", requestID(r))
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return true
}

func mapEnterpriseMemberEmail(w http.ResponseWriter, r *http.Request, d Dependencies, p weknora.Principal) bool {
	if r.Method != "POST" || !strings.HasPrefix(r.URL.Path, "/api/v1/tenants/") {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/tenants/"), "/")
	if len(parts) != 2 || (parts[1] != "members" && parts[1] != "invitations") {
		return true
	}
	tenant, err := strconv.ParseUint(parts[0], 10, 64)
	owner := false
	for _, m := range p.Memberships {
		if m.TenantID == tenant && m.Role == "owner" {
			owner = true
		}
	}
	if err != nil || p.Tenant == nil || p.Tenant.ID != tenant || !owner {
		apierror.Write(w, 403, "workspace.owner_required", "Workspace Owner required", requestID(r))
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
	var body map[string]json.RawMessage
	var email string
	if err != nil || json.Unmarshal(raw, &body) != nil || json.Unmarshal(body["email"], &email) != nil {
		apierror.Write(w, 400, "member.input_invalid", "Invalid member request", requestID(r))
		return false
	}
	if d.Enterprise.Identities == nil {
		apierror.Write(w, 503, "identity.unavailable", "Employee directory unavailable", requestID(r))
		return false
	}
	alias, err := d.Enterprise.Identities.MemberAlias(r.Context(), email)
	if i, installErr := d.Enterprise.Store.Installation(r.Context()); installErr == nil && strings.EqualFold(strings.TrimSpace(email), i.AdminEmail) {
		alias = i.AdminEmail
		err = nil
	}
	if err != nil {
		apierror.Write(w, 409, "member.account_not_registered", "Employee has not signed in or the address is ambiguous", requestID(r))
		return false
	}
	body["email"], _ = json.Marshal(alias)
	raw, _ = json.Marshal(body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return true
}
