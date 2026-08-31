package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
)

type IdentityAdminService interface {
	ChangeStatus(context.Context, string, identity.Status, string, string, string) (identity.Identity, error)
}

func registerIdentityAdminRoutes(mux *http.ServeMux, dependencies Dependencies) {
	for path, status := range map[string]identity.Status{
		"POST /api/v1/mindcreek/identities/{subject}/suspend":  identity.StatusSuspended,
		"POST /api/v1/mindcreek/identities/{subject}/activate": identity.StatusActive,
	} {
		status := status
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			principal, ok := resolvePrincipal(w, r, dependencies.Principals)
			if !ok {
				return
			}
			if principal.User == nil || !principal.User.IsSystemAdmin {
				apierror.Write(w, http.StatusForbidden, "identity.admin_required", "System administrator access is required", requestID(r))
				return
			}
			if dependencies.IdentityAdmin == nil {
				apierror.Write(w, http.StatusServiceUnavailable, "identity.unavailable", "Identity administration is unavailable", requestID(r))
				return
			}
			updated, err := dependencies.IdentityAdmin.ChangeStatus(
				r.Context(), r.PathValue("subject"), status, principal.User.ID, requestID(r), r.RemoteAddr,
			)
			if err != nil {
				writeIdentityAdminError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{
				"subject": updated.BrokerSubject, "status": updated.Status, "local_user_id": updated.LocalUserID,
			}})
		})
	}
}

func writeIdentityAdminError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrNotFound):
		apierror.Write(w, http.StatusNotFound, "identity.not_found", "Corporate identity was not found", requestID(r))
	case errors.Is(err, identity.ErrInvalid):
		apierror.Write(w, http.StatusBadRequest, "identity.invalid_request", "Identity status request is invalid", requestID(r))
	default:
		apierror.Write(w, http.StatusInternalServerError, "identity.operation_failed", "Identity status could not be changed", requestID(r))
	}
}
