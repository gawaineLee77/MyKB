package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/grant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type createGrantRequest struct {
	SubjectType grant.SubjectType `json:"subject_type"`
	SubjectID   string            `json:"subject_id"`
	Permission  grant.Permission  `json:"permission"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

type updateGrantRequest struct {
	ExpectedRevision int64            `json:"expected_revision"`
	Permission       grant.Permission `json:"permission"`
	ExpiresAt        *time.Time       `json:"expires_at,omitempty"`
}

type revokeGrantRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

func registerSharingRoutes(mux *http.ServeMux, dependencies Dependencies) {
	mux.HandleFunc("GET /api/v1/mindcreek/knowledge-bases/{kb_id}/access", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Decisions == nil {
			writeUnavailable(w, r, "authorization.unavailable", "Knowledge-base authorization is unavailable")
			return
		}
		decision, err := dependencies.Decisions.Decide(r.Context(), r.PathValue("kb_id"), authorization.Principal{
			UserID: principal.User.ID, TenantID: principal.Tenant.ID,
		}, r.Header)
		if err != nil {
			if errors.Is(err, authorization.ErrNotFound) || errors.Is(err, authorization.ErrDenied) {
				writeAPIError(w, r, http.StatusNotFound, "resource.not_found", "Resource not found")
				return
			}
			writeUnavailable(w, r, "authorization.unavailable", "Knowledge-base authorization is unavailable")
			return
		}
		if decision.Role == authorization.RoleNone {
			writeAPIError(w, r, http.StatusNotFound, "resource.not_found", "Resource not found")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{
			"knowledge_base_id": decision.KnowledgeBaseID,
			"role":              decision.Role,
			"access_source":     decision.Source,
			"publication_id":    decision.PublicationID,
			"product_mode":      decision.ProductMode,
			"can_read":          decision.Allows(authorization.ActionRead),
			"can_edit_content":  decision.Allows(authorization.ActionEditContent),
			"can_edit_metadata": decision.Role == authorization.RoleOwner || decision.Role == authorization.RoleEditor,
			"can_manage_grants": decision.Allows(authorization.ActionManageGrants),
			"can_delete":        decision.Allows(authorization.ActionDelete),
			"can_publish":       decision.Role == authorization.RoleOwner && decision.ProductMode != "personal_notes",
			"can_download":      decision.Allows(authorization.ActionRead) && decision.Source != authorization.SourceSubscription && decision.Source != authorization.SourceOrganizationPublic,
		}})
	})

	mux.HandleFunc("GET /api/v1/mindcreek/knowledge-bases", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Library == nil {
			writeUnavailable(w, r, "library.unavailable", "Authorized knowledge-base views are unavailable")
			return
		}
		view := library.View(strings.TrimSpace(r.URL.Query().Get("view")))
		if view == "" {
			view = library.ViewOwned
		}
		page, pageErr := positiveQuery(r, "page", 1)
		pageSize, sizeErr := positiveQuery(r, "page_size", 20)
		if pageErr != nil || sizeErr != nil || pageSize > library.MaxPageSize {
			writeBadRequest(w, r, "library.invalid_request", "View or pagination is invalid")
			return
		}
		result, err := dependencies.Library.List(r.Context(), view, page, pageSize, authorization.Principal{
			UserID: principal.User.ID, TenantID: principal.Tenant.ID,
		}, r.Header)
		if err != nil {
			if errors.Is(err, library.ErrInvalid) {
				writeBadRequest(w, r, "library.invalid_request", "View or pagination is invalid")
				return
			}
			writeUnavailable(w, r, "library.unavailable", "Authorized knowledge-base views are unavailable")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/mindcreek/users", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Directory == nil {
			writeUnavailable(w, r, "directory.unavailable", "User directory is unavailable")
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		page, pageErr := positiveQuery(r, "page", 1)
		pageSize, sizeErr := positiveQuery(r, "page_size", 20)
		if len(query) > 120 || pageErr != nil || sizeErr != nil || pageSize > 100 {
			writeBadRequest(w, r, "directory.invalid_request", "User search is invalid")
			return
		}
		result, err := dependencies.Directory.ListTenantMembers(r.Context(), principal.Tenant.ID, query, page, pageSize, r.Header)
		if err != nil {
			writeDirectoryError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/mindcreek/knowledge-bases/{kb_id}/grants", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := resolveGrantActor(w, r, dependencies)
		if !ok {
			return
		}
		items, err := dependencies.Grants.List(r.Context(), r.PathValue("kb_id"), actor, r.Header)
		if err != nil {
			writeGrantError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": items})
	})

	mux.HandleFunc("POST /api/v1/mindcreek/knowledge-bases/{kb_id}/grants", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := resolveGrantActor(w, r, dependencies)
		if !ok {
			return
		}
		var input createGrantRequest
		if !decodeStrictJSON(w, r, &input, 16<<10) {
			return
		}
		result, err := dependencies.Grants.Create(r.Context(), r.PathValue("kb_id"), actor, grant.CreateRequest{
			SubjectType: input.SubjectType, SubjectID: input.SubjectID, Permission: input.Permission,
			ExpiresAt: input.ExpiresAt, CorrelationID: requestID(r),
		}, r.Header)
		if err != nil {
			writeGrantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("PATCH /api/v1/mindcreek/knowledge-bases/{kb_id}/grants/{grant_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := resolveGrantActor(w, r, dependencies)
		if !ok {
			return
		}
		var input updateGrantRequest
		if !decodeStrictJSON(w, r, &input, 16<<10) {
			return
		}
		result, err := dependencies.Grants.Update(r.Context(), r.PathValue("kb_id"), r.PathValue("grant_id"), actor, grant.UpdateRequest{
			ExpectedRevision: input.ExpectedRevision, Permission: input.Permission,
			ExpiresAt: input.ExpiresAt, CorrelationID: requestID(r),
		}, r.Header)
		if err != nil {
			writeGrantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("DELETE /api/v1/mindcreek/knowledge-bases/{kb_id}/grants/{grant_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := resolveGrantActor(w, r, dependencies)
		if !ok {
			return
		}
		var input revokeGrantRequest
		if !decodeStrictJSON(w, r, &input, 8<<10) {
			return
		}
		result, err := dependencies.Grants.Revoke(r.Context(), r.PathValue("kb_id"), r.PathValue("grant_id"), actor, grant.RevokeRequest{
			ExpectedRevision: input.ExpectedRevision, CorrelationID: requestID(r),
		}, r.Header)
		if err != nil {
			writeGrantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})
}

func resolveGrantActor(w http.ResponseWriter, r *http.Request, dependencies Dependencies) (grant.Actor, bool) {
	principal, ok := resolvePrincipal(w, r, dependencies.Principals)
	if !ok {
		return grant.Actor{}, false
	}
	if dependencies.Grants == nil {
		writeUnavailable(w, r, "grant.unavailable", "Knowledge-base grants are unavailable")
		return grant.Actor{}, false
	}
	return grant.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, true
}

func writeGrantError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, grant.ErrInvalid):
		writeBadRequest(w, r, "grant.invalid_request", "Grant request is invalid")
	case errors.Is(err, grant.ErrSubjectUnsupported):
		writeBadRequest(w, r, "grant.subject_unsupported", "Grant subject type is not enabled")
	case errors.Is(err, grant.ErrPersonalNotes):
		writeAPIError(w, r, http.StatusForbidden, "personal_notes.sharing_disabled", "Personal Notes cannot be shared")
	case errors.Is(err, grant.ErrNotOwner), errors.Is(err, grant.ErrNotFound), errors.Is(err, ownership.ErrNotFound):
		writeAPIError(w, r, http.StatusNotFound, "resource.not_found", "Resource not found")
	case errors.Is(err, grant.ErrConflict):
		writeAPIError(w, r, http.StatusConflict, "grant.conflict", "An active grant already exists")
	case errors.Is(err, grant.ErrRevisionConflict):
		writeAPIError(w, r, http.StatusConflict, "grant.revision_conflict", "Grant was changed by another request")
	default:
		writeUnavailable(w, r, "grant.unavailable", "Knowledge-base grants are unavailable")
	}
}

func writeDirectoryError(w http.ResponseWriter, r *http.Request, err error) {
	var upstreamError *weknora.Error
	if errors.As(err, &upstreamError) && upstreamError.Code == "upstream.request_invalid" {
		writeBadRequest(w, r, "directory.invalid_request", "User search is invalid")
		return
	}
	writeUnavailable(w, r, "directory.unavailable", "User directory is unavailable")
}

func writeBadRequest(w http.ResponseWriter, r *http.Request, code, message string) {
	writeAPIError(w, r, http.StatusBadRequest, code, message)
}

func writeUnavailable(w http.ResponseWriter, r *http.Request, code, message string) {
	writeAPIError(w, r, http.StatusServiceUnavailable, code, message)
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	apierror.Write(w, status, code, message, requestID(r))
}
