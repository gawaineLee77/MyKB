package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
)

func registerMemberPreview(mux *http.ServeMux, d Dependencies) {
	mux.HandleFunc("POST /api/v1/mindcreek/members/preview", func(w http.ResponseWriter, r *http.Request) {
		if d.Enterprise == nil || d.Principals == nil {
			apierror.Write(w, 503, "identity.unavailable", "Identity unavailable", requestID(r))
			return
		}
		if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") == "" {
			apierror.Write(w, 403, "auth.human_required", "Human Owner session required", requestID(r))
			return
		}
		p, err := d.Principals.CurrentPrincipal(r.Context(), r.Header)
		if err != nil {
			writePrincipalError(w, r, err)
			return
		}
		var input struct {
			Emails []string `json:"emails"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 160<<10))
		decoder.DisallowUnknownFields()
		var extra any
		if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
			apierror.Write(w, 400, "member.batch_invalid", "Invalid batch", requestID(r))
			return
		}
		rows, err := d.Enterprise.PreviewMembers(r.Context(), p, r.Header, input.Emails)
		if err != nil {
			writeEnterpriseError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"success": true, "data": map[string]any{"tenant_id": p.Tenant.ID, "rows": rows}})
	})
}
