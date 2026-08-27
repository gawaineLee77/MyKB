package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/note"
)

func registerNoteRoutes(mux *http.ServeMux, dependencies Dependencies) {
	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/notes/{note_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		result, err := dependencies.Notes.ListRevisions(r.Context(), r.PathValue("kb_id"), r.PathValue("note_id"), identity)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/notes/{note_id}/revisions/{version}", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		version, err := strconv.Atoi(r.PathValue("version"))
		if err != nil || version < 1 {
			apierror.Write(w, http.StatusBadRequest, "note.invalid_request", "Revision version is invalid", requestID(r))
			return
		}
		result, err := dependencies.Notes.GetRevision(r.Context(), r.PathValue("kb_id"), r.PathValue("note_id"), version, identity)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("POST /api/v1/knowledge-bases/{kb_id}/notes/{note_id}/restore", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		var input note.RestoreInput
		if !decodeStrictJSON(w, r, &input, 8<<10) {
			return
		}
		result, err := dependencies.Notes.Restore(r.Context(), r.PathValue("kb_id"), r.PathValue("note_id"), input, identity, r.Header)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("POST /api/v1/knowledge-bases/{kb_id}/notes/import", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, note.MaxNoteBytes+(32<<10))
		if err := r.ParseMultipartForm(note.MaxNoteBytes + (16 << 10)); err != nil {
			apierror.Write(w, http.StatusBadRequest, "note.import_invalid", "A valid note file is required", requestID(r))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, "note.import_invalid", "A note file is required", requestID(r))
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, note.MaxNoteBytes+1))
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, "note.import_invalid", "Unable to read note file", requestID(r))
			return
		}
		result, err := dependencies.Notes.Import(r.Context(), r.PathValue("kb_id"), header.Filename, content, identity, r.Header)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/notes", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		page, err := positiveQuery(r, "page", 1)
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, "note.invalid_request", "Page is invalid", requestID(r))
			return
		}
		pageSize, err := positiveQuery(r, "page_size", note.MaxPageSize)
		if err != nil || pageSize > note.MaxPageSize {
			apierror.Write(w, http.StatusBadRequest, "note.invalid_request", "Page size is invalid", requestID(r))
			return
		}
		result, err := dependencies.Notes.List(r.Context(), r.PathValue("kb_id"), page, pageSize, identity, r.Header)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("POST /api/v1/knowledge-bases/{kb_id}/notes", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		var input note.WriteInput
		if !decodeStrictJSON(w, r, &input, 128<<10) {
			return
		}
		result, err := dependencies.Notes.Create(r.Context(), r.PathValue("kb_id"), input, identity, r.Header)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/notes/{note_id}", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		result, err := dependencies.Notes.Get(r.Context(), r.PathValue("kb_id"), r.PathValue("note_id"), identity, r.Header)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("PATCH /api/v1/knowledge-bases/{kb_id}/notes/{note_id}", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		var input note.WriteInput
		if !decodeStrictJSON(w, r, &input, 128<<10) {
			return
		}
		result, err := dependencies.Notes.Update(r.Context(), r.PathValue("kb_id"), r.PathValue("note_id"), input, identity, r.Header)
		if err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("DELETE /api/v1/knowledge-bases/{kb_id}/notes/{note_id}", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := resolveNoteIdentity(w, r, dependencies)
		if !ok {
			return
		}
		if err := dependencies.Notes.Delete(r.Context(), r.PathValue("kb_id"), r.PathValue("note_id"), identity, r.Header); err != nil {
			writeNoteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	})
}

func resolveNoteIdentity(w http.ResponseWriter, r *http.Request, dependencies Dependencies) (access.Identity, bool) {
	principal, ok := resolvePrincipal(w, r, dependencies.Principals)
	if !ok {
		return access.Identity{}, false
	}
	if dependencies.Notes == nil {
		apierror.Write(w, http.StatusServiceUnavailable, "note.unavailable", "Notes service is unavailable", requestID(r))
		return access.Identity{}, false
	}
	return access.Identity{UserID: principal.User.ID, TenantID: principal.Tenant.ID}, true
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, destination any, maxBytes int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body is not valid", requestID(r))
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body must contain one JSON document", requestID(r))
		return false
	}
	return true
}

func positiveQuery(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, errors.New("query parameter must be positive")
	}
	return value, nil
}

func writeNoteError(w http.ResponseWriter, r *http.Request, err error) {
	var productError *note.Error
	if errors.As(err, &productError) {
		apierror.Write(w, productError.StatusCode, productError.Code, productError.Message, requestID(r))
		return
	}
	apierror.Write(w, http.StatusInternalServerError, "note.operation_failed", "Notes operation failed", requestID(r))
}
