package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/catalog"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
)

type unpublishRequest struct {
	ExpectedRowVersion int64 `json:"expected_row_version"`
}

func registerPublicationRoutes(mux *http.ServeMux, dependencies Dependencies) {
	mux.HandleFunc("GET /api/v1/mindcreek/knowledge-bases/{kb_id}/publication", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Publications == nil {
			writeUnavailable(w, r, "publication.unavailable", "Publication service is unavailable")
			return
		}
		result, err := dependencies.Publications.GetForOwner(r.Context(), r.PathValue("kb_id"), publication.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, r.Header)
		if err != nil {
			writePublicationError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("POST /api/v1/mindcreek/knowledge-bases/{kb_id}/publication", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Publications == nil {
			writeUnavailable(w, r, "publication.unavailable", "Publication service is unavailable")
			return
		}
		var input publication.WriteRequest
		if !decodeStrictJSON(w, r, &input, 32<<10) {
			return
		}
		input.CorrelationID = requestID(r)
		result, err := dependencies.Publications.Publish(r.Context(), r.PathValue("kb_id"), publication.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, input, r.Header)
		if err != nil {
			writePublicationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("PATCH /api/v1/mindcreek/knowledge-bases/{kb_id}/publication", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Publications == nil {
			writeUnavailable(w, r, "publication.unavailable", "Publication service is unavailable")
			return
		}
		var input publication.WriteRequest
		if !decodeStrictJSON(w, r, &input, 32<<10) {
			return
		}
		input.CorrelationID = requestID(r)
		result, err := dependencies.Publications.Update(r.Context(), r.PathValue("kb_id"), publication.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, input, r.Header)
		if err != nil {
			writePublicationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("DELETE /api/v1/mindcreek/knowledge-bases/{kb_id}/publication", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Publications == nil {
			writeUnavailable(w, r, "publication.unavailable", "Publication service is unavailable")
			return
		}
		var input unpublishRequest
		if !decodeStrictJSON(w, r, &input, 8<<10) {
			return
		}
		result, err := dependencies.Publications.Unpublish(r.Context(), r.PathValue("kb_id"), publication.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, input.ExpectedRowVersion, requestID(r), r.Header)
		if err != nil {
			writePublicationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/mindcreek/catalog", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Catalog == nil {
			writeUnavailable(w, r, "catalog.unavailable", "Publication catalog is unavailable")
			return
		}
		page, pageErr := positiveQuery(r, "page", 1)
		pageSize, sizeErr := positiveQuery(r, "page_size", 20)
		filter := catalog.Filter{Query: strings.TrimSpace(r.URL.Query().Get("q")), Tag: strings.TrimSpace(r.URL.Query().Get("tag")), Owner: strings.TrimSpace(r.URL.Query().Get("owner")), AccessMode: publication.AccessMode(strings.TrimSpace(r.URL.Query().Get("access_mode"))), Page: page, PageSize: pageSize}
		if raw := strings.TrimSpace(r.URL.Query().Get("updated_after")); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeBadRequest(w, r, "catalog.invalid_request", "Catalog filter is invalid")
				return
			}
			filter.UpdatedAfter = &parsed
		}
		if pageErr != nil || sizeErr != nil {
			writeBadRequest(w, r, "catalog.invalid_request", "Catalog filter is invalid")
			return
		}
		result, err := dependencies.Catalog.List(r.Context(), catalog.Principal{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, filter)
		if err != nil {
			writeCatalogError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/mindcreek/publications/{publication_id}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Catalog == nil {
			writeUnavailable(w, r, "catalog.unavailable", "Publication catalog is unavailable")
			return
		}
		result, err := dependencies.Catalog.Get(r.Context(), catalog.Principal{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, r.PathValue("publication_id"))
		if err != nil {
			writeCatalogError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/mindcreek/me/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Subscriptions == nil {
			writeUnavailable(w, r, "subscription.unavailable", "Subscription service is unavailable")
			return
		}
		page, pageErr := positiveQuery(r, "page", 1)
		pageSize, sizeErr := positiveQuery(r, "page_size", 20)
		if pageErr != nil || sizeErr != nil || pageSize > 100 {
			writeBadRequest(w, r, "subscription.invalid_request", "Subscription pagination is invalid")
			return
		}
		items, err := dependencies.Subscriptions.List(r.Context(), subscription.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID})
		if err != nil {
			writeSubscriptionError(w, r, err)
			return
		}
		start := len(items)
		if page-1 <= len(items)/pageSize {
			start = (page - 1) * pageSize
		}
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"items": items[start:end], "total": len(items), "page": page, "page_size": pageSize}})
	})

	for method, action := range map[string]string{http.MethodPost: "subscribe", http.MethodDelete: "unsubscribe"} {
		method, action := method, action
		mux.HandleFunc(method+" /api/v1/mindcreek/publications/{publication_id}/subscription", func(w http.ResponseWriter, r *http.Request) {
			principal, ok := resolvePrincipal(w, r, dependencies.Principals)
			if !ok {
				return
			}
			if dependencies.Subscriptions == nil {
				writeUnavailable(w, r, "subscription.unavailable", "Subscription service is unavailable")
				return
			}
			actor := subscription.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}
			var result subscription.Result
			var err error
			if action == "subscribe" {
				result, err = dependencies.Subscriptions.Subscribe(r.Context(), r.PathValue("publication_id"), actor, requestID(r))
			} else {
				result, err = dependencies.Subscriptions.Unsubscribe(r.Context(), r.PathValue("publication_id"), actor, requestID(r))
			}
			if errors.Is(err, subscription.ErrNotFound) && action == "unsubscribe" {
				writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"changed": false}})
				return
			}
			if err != nil {
				writeSubscriptionError(w, r, err)
				return
			}
			status := http.StatusOK
			if action == "subscribe" && result.Changed {
				status = http.StatusCreated
			}
			writeJSON(w, status, map[string]any{"success": true, "data": result})
		})
	}

	mux.HandleFunc("POST /api/v1/mindcreek/publications/{publication_id}/mark-seen", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Subscriptions == nil {
			writeUnavailable(w, r, "subscription.unavailable", "Subscription service is unavailable")
			return
		}
		result, err := dependencies.Subscriptions.MarkSeen(r.Context(), r.PathValue("publication_id"), subscription.Actor{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, requestID(r))
		if err != nil {
			writeSubscriptionError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})
}

func writePublicationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, publication.ErrInvalid):
		writeBadRequest(w, r, "publication.invalid_request", "Publication request is invalid")
	case errors.Is(err, publication.ErrPersonalNotes):
		writeAPIError(w, r, http.StatusForbidden, "personal_notes.publication_disabled", "Personal Notes cannot be published")
	case errors.Is(err, publication.ErrConflict):
		writeAPIError(w, r, http.StatusConflict, "publication.conflict", "Knowledge base is already published")
	case errors.Is(err, publication.ErrRevisionConflict):
		writeAPIError(w, r, http.StatusConflict, "publication.revision_conflict", "Publication was changed by another request")
	case errors.Is(err, publication.ErrNotFound), errors.Is(err, publication.ErrNotOwner), errors.Is(err, ownership.ErrNotFound):
		writeAPIError(w, r, http.StatusNotFound, "resource.not_found", "Resource not found")
	default:
		writeUnavailable(w, r, "publication.unavailable", "Publication service is unavailable")
	}
}

func writeCatalogError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, catalog.ErrInvalid) {
		writeBadRequest(w, r, "catalog.invalid_request", "Catalog request is invalid")
		return
	}
	if errors.Is(err, catalog.ErrNotFound) {
		writeAPIError(w, r, http.StatusNotFound, "publication.unavailable", "Publication is unavailable")
		return
	}
	writeUnavailable(w, r, "catalog.unavailable", "Publication catalog is unavailable")
}

func writeSubscriptionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, subscription.ErrInvalid):
		writeBadRequest(w, r, "subscription.invalid_request", "Subscription request is invalid")
	case errors.Is(err, subscription.ErrOwner):
		writeAPIError(w, r, http.StatusConflict, "subscription.owner_not_required", "Owners do not subscribe to their own publication")
	case errors.Is(err, subscription.ErrPublication), errors.Is(err, subscription.ErrOutsideAudience), errors.Is(err, subscription.ErrNotFound):
		writeAPIError(w, r, http.StatusNotFound, "publication.unavailable", "Publication is unavailable")
	default:
		writeUnavailable(w, r, "subscription.unavailable", "Subscription service is unavailable")
	}
}
