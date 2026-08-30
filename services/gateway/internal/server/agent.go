package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentscope"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
)

func registerAgentRoutes(mux *http.ServeMux, dependencies Dependencies) {
	mux.HandleFunc("GET /api/v1/mindcreek/agent/scope", func(w http.ResponseWriter, r *http.Request) {
		resolveAgentScope(w, r, dependencies, agentscope.Request{Selection: agentscope.SelectionDefault})
	})
	mux.HandleFunc("POST /api/v1/mindcreek/agent/scope/resolve", func(w http.ResponseWriter, r *http.Request) {
		var input agentscope.Request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body is not valid", requestID(r))
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body must contain one JSON document", requestID(r))
			return
		}
		resolveAgentScope(w, r, dependencies, input)
	})
}

func resolveAgentScope(w http.ResponseWriter, r *http.Request, dependencies Dependencies, input agentscope.Request) {
	principal, ok := resolvePrincipal(w, r, dependencies.Principals)
	if !ok {
		return
	}
	if dependencies.AgentScopes == nil {
		apierror.Write(w, http.StatusServiceUnavailable, "agent.scope_unavailable", "Agent scope is unavailable", requestID(r))
		return
	}
	result, err := dependencies.AgentScopes.Resolve(r.Context(), input,
		authorization.Principal{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, r.Header)
	if err != nil {
		writeAgentScopeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func writeAgentScopeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, agentscope.ErrDenied):
		apierror.Write(w, http.StatusNotFound, "resource.not_found", "Resource not found", requestID(r))
	case errors.Is(err, agentscope.ErrInvalid):
		apierror.Write(w, http.StatusBadRequest, "agent.scope_invalid", "Agent scope is invalid", requestID(r))
	case errors.Is(err, agentscope.ErrTooLarge):
		apierror.Write(w, http.StatusUnprocessableEntity, "agent.scope_too_large", "Select fewer knowledge bases", requestID(r))
	default:
		apierror.Write(w, http.StatusServiceUnavailable, "agent.scope_unavailable", "Agent scope is unavailable", requestID(r))
	}
}
