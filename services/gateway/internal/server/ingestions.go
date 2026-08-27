package server

import (
	"errors"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ingestion"
)

const maxIngestionRequestBytes = (50 << 20) + (1 << 20)

func registerIngestionRoutes(mux *http.ServeMux, dependencies Dependencies) {
	mux.HandleFunc("POST /api/v1/knowledge-bases/{kb_id}/ingestions", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveIngestionIdentity(w, r, dependencies)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxIngestionRequestBytes)
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			apierror.Write(w, http.StatusBadRequest, "ingestion.upload_invalid", "A valid document file is required", requestID(r))
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, "ingestion.upload_invalid", "A document file is required", requestID(r))
			return
		}
		defer file.Close()
		result, err := dependencies.Ingestions.Upload(r.Context(), r.PathValue("kb_id"), header.Filename, header.Size, file, identity, r.Header)
		if err != nil {
			writeIngestionError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/ingestions", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveIngestionIdentity(w, r, dependencies)
		if !ok {
			return
		}
		page, err := positiveQuery(r, "page", 1)
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, "ingestion.invalid_request", "Page is invalid", requestID(r))
			return
		}
		pageSize, err := positiveQuery(r, "page_size", 20)
		if err != nil || pageSize > ingestion.MaxPageSize {
			apierror.Write(w, http.StatusBadRequest, "ingestion.invalid_request", "Page size is invalid", requestID(r))
			return
		}
		result, err := dependencies.Ingestions.List(r.Context(), r.PathValue("kb_id"), page, pageSize, identity, r.Header)
		if err != nil {
			writeIngestionError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/ingestions/{ingestion_id}", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveIngestionIdentity(w, r, dependencies)
		if !ok {
			return
		}
		result, err := dependencies.Ingestions.Get(r.Context(), r.PathValue("kb_id"), r.PathValue("ingestion_id"), identity, r.Header)
		if err != nil {
			writeIngestionError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	registerIngestionMutation(mux, dependencies, "retry", func(r *http.Request, identity access.Identity) (any, error) {
		return dependencies.Ingestions.Retry(r.Context(), r.PathValue("kb_id"), r.PathValue("ingestion_id"), identity, r.Header)
	})
	registerIngestionMutation(mux, dependencies, "cancel", func(r *http.Request, identity access.Identity) (any, error) {
		return dependencies.Ingestions.Cancel(r.Context(), r.PathValue("kb_id"), r.PathValue("ingestion_id"), identity, r.Header)
	})
}

func registerIngestionMutation(mux *http.ServeMux, dependencies Dependencies, action string, operation func(*http.Request, access.Identity) (any, error)) {
	mux.HandleFunc("POST /api/v1/knowledge-bases/{kb_id}/ingestions/{ingestion_id}/"+action, func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveIngestionIdentity(w, r, dependencies)
		if !ok {
			return
		}
		result, err := operation(r, identity)
		if err != nil {
			writeIngestionError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "data": result})
	})
}

func resolveIngestionIdentity(w http.ResponseWriter, r *http.Request, dependencies Dependencies) (access.Identity, bool) {
	principal, ok := resolvePrincipal(w, r, dependencies.Principals)
	if !ok {
		return access.Identity{}, false
	}
	if dependencies.Ingestions == nil {
		apierror.Write(w, http.StatusServiceUnavailable, "ingestion.unavailable", "Document ingestion service is unavailable", requestID(r))
		return access.Identity{}, false
	}
	return access.Identity{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, true
}

func writeIngestionError(w http.ResponseWriter, r *http.Request, err error) {
	var productError *ingestion.Error
	if errors.As(err, &productError) {
		apierror.Write(w, productError.StatusCode, productError.Code, productError.Message, requestID(r))
		return
	}
	apierror.Write(w, http.StatusInternalServerError, "ingestion.operation_failed", "Document ingestion operation failed", requestID(r))
}
