package agenticon

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/safeimage"
)

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/mindcreek/agent-icons", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, safeimage.MaxBytes))
		if err != nil {
			nativeaccess.WriteError(w, r, fail(413, "agent_icon.image_too_large"))
			return
		}
		ref, err := s.Upload(r.Context(), r.Header, raw)
		if err != nil {
			nativeaccess.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]string{"avatar": ref}})
	})
	mux.HandleFunc("GET /api/v1/mindcreek/agent-icons/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		png, err := s.Read(r.Context(), r.Header, r.PathValue("id"), r.URL.Query().Get("agent_id"))
		if err != nil {
			nativeaccess.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
}
