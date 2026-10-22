package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
)

// WorkspaceAdminService keeps the product authorization boundary testable
// without exposing delegated credentials to HTTP handlers or responses.
type WorkspaceAdminService interface {
	SetDelegation(context.Context, weknora.Principal, http.Header, uint64, workspaceadmin.DelegationInput) (workspaceadmin.DelegationStatus, error)
	GetDelegationStatus(context.Context, weknora.Principal, http.Header, uint64) (workspaceadmin.DelegationStatus, error)
	RevokeDelegation(context.Context, weknora.Principal, http.Header, uint64) error
	ListKeys(context.Context, weknora.Principal, http.Header, uint64) (json.RawMessage, error)
	CreateKey(context.Context, weknora.Principal, http.Header, uint64, json.RawMessage) (json.RawMessage, error)
	UpdateKey(context.Context, weknora.Principal, http.Header, uint64, uint64, json.RawMessage) (json.RawMessage, error)
	RevokeKey(context.Context, weknora.Principal, http.Header, uint64, uint64) error
	Manage(context.Context, weknora.Principal, http.Header, uint64, string, string, json.RawMessage) (json.RawMessage, error)
	ManageMachine(context.Context, http.Header, uint64, string, string, json.RawMessage) (json.RawMessage, error)
	SelfLeave(context.Context, weknora.Principal, http.Header, uint64) (json.RawMessage, error)
	DisableCreatorKeys(context.Context, string) error
	CheckKeyUse(context.Context, string, uint64) error
}

func writeWorkspaceAdminError(w http.ResponseWriter, r *http.Request, err error) {
	var e *workspaceadmin.Error
	if errors.As(err, &e) {
		apierror.Write(w, e.Status, e.Code, "Workspace management could not be completed", requestID(r))
		return
	}
	writeEnterpriseError(w, r, err)
}

func workspaceAdminPrincipal(w http.ResponseWriter, r *http.Request) (weknora.Principal, bool) {
	p, ok := principalFromRequest(r)
	if !ok || r.Header.Get("Authorization") == "" || r.Header.Get("X-API-Key") != "" || p.User == nil || p.Tenant == nil {
		apierror.Write(w, 403, "auth.human_required", "Human workspace manager required", requestID(r))
		return p, false
	}
	return p, true
}

func workspaceAdminBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || !json.Valid(raw) {
		apierror.Write(w, 400, "workspace.input_invalid", "Invalid workspace request", requestID(r))
		return nil, false
	}
	return raw, true
}

func registerWorkspaceAdminRoutes(mux *http.ServeMux, d Dependencies, fallback http.Handler) {
	if d.WorkspaceAdmin == nil {
		return
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		mux.HandleFunc(method+" /api/v1/mindcreek/workspace-admin/delegation", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			p, ok := workspaceAdminPrincipal(w, r)
			if !ok {
				return
			}
			var result any
			var err error
			switch method {
			case "GET":
				result, err = d.WorkspaceAdmin.GetDelegationStatus(r.Context(), p, r.Header, p.Tenant.ID)
			case "POST":
				raw, ok := workspaceAdminBody(w, r)
				if !ok {
					return
				}
				var fields map[string]json.RawMessage
				if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != 0 {
					apierror.Write(w, 400, "workspace.input_invalid", "Delegation takes no credentials or role input", requestID(r))
					return
				}
				result, err = d.WorkspaceAdmin.SetDelegation(r.Context(), p, r.Header, p.Tenant.ID, workspaceadmin.DelegationInput{})
			case "DELETE":
				err = d.WorkspaceAdmin.RevokeDelegation(r.Context(), p, r.Header, p.Tenant.ID)
			}
			if err != nil {
				writeWorkspaceAdminError(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]any{"success": true, "data": result})
		})
	}
	for _, route := range []string{
		"GET /api/v1/tenants/{tenant}/api-keys", "POST /api/v1/tenants/{tenant}/api-keys",
		"PUT /api/v1/tenants/{tenant}/api-keys/{key}", "DELETE /api/v1/tenants/{tenant}/api-keys/{key}",
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			p, ok := workspaceAdminPrincipal(w, r)
			if !ok {
				return
			}
			tenant, err := strconv.ParseUint(r.PathValue("tenant"), 10, 64)
			if err != nil || tenant == 0 {
				apierror.Write(w, 400, "workspace.input_invalid", "Invalid workspace ID", requestID(r))
				return
			}
			keyID := uint64(0)
			if v := r.PathValue("key"); v != "" {
				keyID, err = strconv.ParseUint(v, 10, 64)
			}
			if err != nil || tenant == 0 || (r.PathValue("key") != "" && keyID == 0) {
				apierror.Write(w, 400, "workspace.input_invalid", "Invalid workspace or key ID", requestID(r))
				return
			}
			var raw json.RawMessage
			if r.Method == "POST" || r.Method == "PUT" {
				raw, ok = workspaceAdminBody(w, r)
				if !ok {
					return
				}
			}
			var result json.RawMessage
			status := 200
			switch r.Method {
			case "GET":
				result, err = d.WorkspaceAdmin.ListKeys(r.Context(), p, r.Header, tenant)
			case "POST":
				result, err = d.WorkspaceAdmin.CreateKey(r.Context(), p, r.Header, tenant, raw)
				status = 201
			case "PUT":
				result, err = d.WorkspaceAdmin.UpdateKey(r.Context(), p, r.Header, tenant, keyID, raw)
			case "DELETE":
				err = d.WorkspaceAdmin.RevokeKey(r.Context(), p, r.Header, tenant, keyID)
				result = json.RawMessage(`{"success":true}`)
			}
			if err != nil {
				writeWorkspaceAdminError(w, r, err)
				return
			}
			writeJSON(w, status, result)
		})
	}
	for _, route := range []string{
		"PUT /api/v1/tenants/{tenant}",
		"POST /api/v1/tenants/{tenant}/leave",
		"POST /api/v1/tenants/{tenant}/members", "PUT /api/v1/tenants/{tenant}/members/{member}", "DELETE /api/v1/tenants/{tenant}/members/{member}",
		"POST /api/v1/tenants/{tenant}/invitations", "DELETE /api/v1/tenants/{tenant}/invitations/{invitation}",
		"POST /api/v1/tenants/{tenant}/invite-links",
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/tenants/"+r.PathValue("tenant"))
			machine := r.Header.Get("X-API-Key") != ""
			// Native authorization still governs machine operations. Membership
			// mutations additionally share the role-change lock and key revocation.
			if machine && (suffix == "" || suffix == "/leave") {
				fallback.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			var p weknora.Principal
			var ok bool
			if !machine {
				p, ok = workspaceAdminPrincipal(w, r)
				if !ok {
					return
				}
			}
			tenant, err := strconv.ParseUint(r.PathValue("tenant"), 10, 64)
			if err != nil || tenant == 0 {
				apierror.Write(w, 400, "workspace.input_invalid", "Invalid workspace ID", requestID(r))
				return
			}
			var raw json.RawMessage
			if r.Method != "DELETE" && !strings.HasSuffix(r.URL.Path, "/leave") {
				raw, ok = workspaceAdminBody(w, r)
				if !ok {
					return
				}
			}
			var result json.RawMessage
			if machine {
				result, err = d.WorkspaceAdmin.ManageMachine(r.Context(), r.Header, tenant, r.Method, suffix, raw)
			} else if suffix == "/leave" {
				result, err = d.WorkspaceAdmin.SelfLeave(r.Context(), p, r.Header, tenant)
			} else {
				result, err = d.WorkspaceAdmin.Manage(r.Context(), p, r.Header, tenant, r.Method, suffix, raw)
			}
			if err != nil {
				writeWorkspaceAdminError(w, r, err)
				return
			}
			status := 200
			if r.Method == "POST" && suffix != "/leave" {
				status = 201
			}
			writeJSON(w, status, result)
		})
	}
}

func checkWorkspaceKeyUse(w http.ResponseWriter, r *http.Request, d Dependencies) bool {
	if r.Header.Get("Authorization") != "" {
		apierror.Write(w, 400, "auth.ambiguous_credentials", "Use one authentication credential", requestID(r))
		return false
	}
	tenant := uint64(0)
	if requested := r.Header.Get("X-Tenant-ID"); requested != "" {
		var err error
		tenant, err = strconv.ParseUint(requested, 10, 64)
		if err != nil || tenant == 0 {
			apierror.Write(w, 403, "workspace.denied", "Invalid workspace ID", requestID(r))
			return false
		}
	}
	if err := d.WorkspaceAdmin.CheckKeyUse(r.Context(), r.Header.Get("X-API-Key"), tenant); err != nil {
		writeWorkspaceAdminError(w, r, err)
		return false
	}
	return true
}
