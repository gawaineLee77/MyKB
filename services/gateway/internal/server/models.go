package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

func registerManagedModelRoutes(mux *http.ServeMux, dependencies Dependencies) {
	mux.HandleFunc("GET /api/v1/mindcreek/models", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Models == nil {
			apierror.Write(w, http.StatusServiceUnavailable, "models.unavailable", "Model service is unavailable", requestID(r))
			return
		}
		result, err := dependencies.Models.Snapshot(r.Context(), principal, r.Header)
		if err != nil {
			writeManagedModelError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("POST /api/v1/mindcreek/models/{model_id}/test", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Models == nil {
			apierror.Write(w, http.StatusServiceUnavailable, "models.unavailable", "Model service is unavailable", requestID(r))
			return
		}
		result, err := dependencies.Models.TestManaged(r.Context(), r.PathValue("model_id"), principal, r.Header)
		if err != nil {
			writeManagedModelError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("POST /api/v1/mindcreek/models/overrides", func(w http.ResponseWriter, r *http.Request) {
		principal, input, ok := decodeModelInput(w, r, dependencies)
		if !ok {
			return
		}
		result, err := dependencies.Models.CreateOverride(r.Context(), input, principal, r.Header)
		if err != nil {
			writeManagedModelError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("PUT /api/v1/mindcreek/models/overrides/{model_id}", func(w http.ResponseWriter, r *http.Request) {
		principal, input, ok := decodeModelInput(w, r, dependencies)
		if !ok {
			return
		}
		result, err := dependencies.Models.UpdateOverride(r.Context(), r.PathValue("model_id"), input, principal, r.Header)
		if err != nil {
			writeManagedModelError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	mux.HandleFunc("DELETE /api/v1/mindcreek/models/overrides/{model_id}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Models == nil {
			apierror.Write(w, http.StatusServiceUnavailable, "models.unavailable", "Model service is unavailable", requestID(r))
			return
		}
		if err := dependencies.Models.DeleteOverride(r.Context(), r.PathValue("model_id"), principal, r.Header); err != nil {
			writeManagedModelError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/mindcreek/models/overrides/test", func(w http.ResponseWriter, r *http.Request) {
		principal, input, ok := decodeModelInput(w, r, dependencies)
		if !ok {
			return
		}
		result, err := dependencies.Models.TestOverride(r.Context(), input, strings.TrimSpace(r.URL.Query().Get("model_id")), principal, r.Header)
		if err != nil {
			writeManagedModelError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})

	// Once the Phase 5 model service is active, every browser-side mutation or
	// provider connection test must use the governed facade above. The adapter
	// still reaches these private upstream endpoints directly, so this closes a
	// policy bypass without modifying WeKnora or breaking safe model reads.
	if dependencies.Models != nil {
		deny := func(w http.ResponseWriter, r *http.Request) {
			apierror.Write(w, http.StatusNotFound, "models.raw_route_disabled", "Use MindCreek Advanced Settings for model overrides", requestID(r))
		}
		mux.HandleFunc("POST /api/v1/models", deny)
		mux.HandleFunc("PUT /api/v1/models/{model_id}", deny)
		mux.HandleFunc("DELETE /api/v1/models/{model_id}", deny)
		mux.HandleFunc("POST /api/v1/models/{model_id}/debug", deny)
		mux.HandleFunc("PUT /api/v1/models/{model_id}/credentials", deny)
		mux.HandleFunc("DELETE /api/v1/models/{model_id}/credentials/{field}", deny)
		mux.HandleFunc("POST /api/v1/initialization/remote/check", deny)
		mux.HandleFunc("POST /api/v1/initialization/embedding/test", deny)
		mux.HandleFunc("POST /api/v1/initialization/rerank/check", deny)
		mux.HandleFunc("POST /api/v1/initialization/ollama/models/check", deny)
		mux.HandleFunc("POST /api/v1/initialization/ollama/models/download", deny)
	}
}

func decodeModelInput(w http.ResponseWriter, r *http.Request, dependencies Dependencies) (weknora.Principal, managedmodel.OverrideInput, bool) {
	principal, ok := resolvePrincipal(w, r, dependencies.Principals)
	if !ok {
		return weknora.Principal{}, managedmodel.OverrideInput{}, false
	}
	if dependencies.Models == nil {
		apierror.Write(w, http.StatusServiceUnavailable, "models.unavailable", "Model service is unavailable", requestID(r))
		return weknora.Principal{}, managedmodel.OverrideInput{}, false
	}
	var input managedmodel.OverrideInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body is not valid", requestID(r))
		return weknora.Principal{}, managedmodel.OverrideInput{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body must contain one JSON document", requestID(r))
		return weknora.Principal{}, managedmodel.OverrideInput{}, false
	}
	return principal, input, true
}

func writeManagedModelError(w http.ResponseWriter, r *http.Request, err error) {
	var productError *managedmodel.Error
	if errors.As(err, &productError) {
		apierror.Write(w, productError.StatusCode, productError.Code, productError.Message, requestID(r))
		return
	}
	apierror.Write(w, http.StatusInternalServerError, "models.operation_failed", "Model operation failed", requestID(r))
}
