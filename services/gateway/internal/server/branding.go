package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/branding"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/safeimage"
)

func publicBrandingRequest(r *http.Request) bool {
	return (r.Method == "GET" || r.Method == "HEAD") && (r.URL.Path == branding.Route || r.URL.Path == branding.Route+"/logo")
}

func brandingError(w http.ResponseWriter, r *http.Request, err error) {
	var e *branding.Error
	if errors.As(err, &e) {
		apierror.Write(w, e.Status, e.Code, "Brand settings could not be updated", requestID(r))
		return
	}
	apierror.Write(w, 503, "branding.unavailable", "Brand settings unavailable", requestID(r))
}

func registerBrandingRoutes(mux *http.ServeMux, d Dependencies) {
	if d.Branding == nil {
		return
	}
	mux.HandleFunc("GET "+branding.Route, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		value, err := d.Branding.Get(r.Context())
		if err != nil {
			brandingError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "data": value.Snapshot()})
	})
	mux.HandleFunc("GET "+branding.Route+"/logo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		value, err := d.Branding.Get(r.Context())
		if err != nil {
			brandingError(w, r, err)
			return
		}
		if len(value.Logo) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(value.Logo)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.WriteHeader(200)
		if r.Method != "HEAD" {
			_, _ = w.Write(value.Logo)
		}
	})
	mux.HandleFunc("PUT "+branding.Route, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		p, ok := principalFromRequest(r)
		if !ok || r.Header.Get("Authorization") == "" || r.Header.Get("X-API-Key") != "" {
			apierror.Write(w, 403, "branding.installation_admin_required", "Installation administrator session required", requestID(r))
			return
		}
		// Reject before decoding images; workspace roles never authorize branding.
		if err := d.Branding.CanManage(r.Context(), p); err != nil {
			brandingError(w, r, err)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, safeimage.MaxDataURLBytes+4096))
		decoder.DisallowUnknownFields()
		var input branding.Input
		var extra any
		if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
			apierror.Write(w, 400, "branding.input_invalid", "Invalid brand settings", requestID(r))
			return
		}
		value, err := d.Branding.Update(r.Context(), p, input, requestID(r))
		if err != nil {
			brandingError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "data": value.Snapshot()})
	})
}
