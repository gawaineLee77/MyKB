package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
)

type knowledgeExecutionService interface {
	SetKnowledgeDelegation(context.Context, weknora.Principal, http.Header, uint64) (workspaceadmin.DelegationStatus, error)
	GetKnowledgeDelegationStatus(context.Context, weknora.Principal, http.Header, uint64) (workspaceadmin.DelegationStatus, error)
	RevokeKnowledgeDelegation(context.Context, weknora.Principal, http.Header, uint64) error
}

func registerKnowledgePermissions(mux *http.ServeMux, d Dependencies) {
	if d.KBPermissions != nil {
		d.KBPermissions.RegisterRoutes(mux)
	}
	if d.KBGuard != nil {
		mux.Handle("POST /api/v1/mindcreek/kb-permissions/{kb}/export-chunks", d.KBGuard.Wrap(http.NotFoundHandler()))
	}
	s, ok := d.WorkspaceAdmin.(knowledgeExecutionService)
	if !ok {
		return
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		mux.HandleFunc(method+" /api/v1/mindcreek/kb-collaboration/delegation", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			p, ok := workspaceAdminPrincipal(w, r)
			if !ok {
				return
			}
			var result any
			var err error
			switch method {
			case "GET":
				result, err = s.GetKnowledgeDelegationStatus(r.Context(), p, r.Header, p.Tenant.ID)
			case "POST":
				raw, ok := workspaceAdminBody(w, r)
				if !ok {
					return
				}
				var body map[string]json.RawMessage
				if json.Unmarshal(raw, &body) != nil || body == nil || len(body) != 0 {
					nativeaccess.WriteError(w, r, &nativeaccess.Error{Status: 400, Code: "kb_acl.delegation_input_invalid"})
					return
				}
				result, err = s.SetKnowledgeDelegation(r.Context(), p, r.Header, p.Tenant.ID)
			case "DELETE":
				err = s.RevokeKnowledgeDelegation(r.Context(), p, r.Header, p.Tenant.ID)
			}
			if err != nil {
				writeWorkspaceAdminError(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]any{"success": true, "data": result})
		})
	}
}
