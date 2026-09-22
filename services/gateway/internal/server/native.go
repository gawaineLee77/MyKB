package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/policy"
)

// newNativeHandler deliberately does not register legacy business handlers.
// Their code and migrations remain available to historical release checks.
func newNativeHandler(cfg config.Config, caps *capability.Registry, d Dependencies, fallback http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"service": "mindcreek-gateway", "status": "ok"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"service": "mindcreek-gateway", "version": cfg.ProductVersion, "compatible_weknora_version": cfg.UpstreamVersion})
	})
	if caps != nil {
		mux.HandleFunc("GET /api/v1/capabilities/knowledge-modes", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, caps.Document(cfg.ProductVersion, cfg.UpstreamVersion))
		})
	}
	registerEnterpriseRoutes(mux, d)
	registerManagedModelRoutes(mux, d)
	registerNativeScopeRoutes(mux, d)
	registerMemberPreview(mux, d)
	if d.Assistant != nil {
		mux.Handle("/api/v1/mindcreek/assistant/", d.Assistant.Handler(d.AssistantProxy))
		mux.HandleFunc("GET /api/v1/mindcreek/assistant-frame-policy", d.Assistant.FramePolicy)
	}
	if d.IdentityAdmin != nil {
		registerIdentityAdminRoutes(mux, d)
	}
	if d.IdentityBroker != nil {
		mux.Handle("/api/v1/mindcreek/oidc/", d.IdentityBroker)
	}
	if d.MCP != nil {
		mux.Handle("/mcp", d.MCP)
	}
	if d.Observability != nil {
		mux.Handle("GET /internal/metrics", d.Observability)
	}
	mux.Handle("/", fallback)
	handler := enterpriseIdentityMiddleware(mux, d)
	// Retired namespace errors are stable even for clients whose old session
	// no longer exists; the response contains no resource or identity data.
	guard := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, err := policy.NormalizeRequestPath(r)
		if err != nil {
			nativeaccess.WriteError(w, r, &nativeaccess.Error{Status: 400, Code: "request.path_invalid"})
			return
		}
		if nativeaccess.Retired(path) {
			nativeaccess.WriteError(w, r, &nativeaccess.Error{Status: 410, Code: "feature.retired"})
			return
		}
		// These inherited product facades authorize a human manager. Native
		// auth/me may return a workspace Owner as the user for an API key, which
		// must never grant platform identity or product model administration.
		if r.Header.Get("X-API-Key") != "" && (strings.HasPrefix(path, "/api/v1/mindcreek/identities/") || path == "/api/v1/mindcreek/models" || strings.HasPrefix(path, "/api/v1/mindcreek/models/")) {
			nativeaccess.WriteError(w, r, &nativeaccess.Error{Status: 403, Code: "auth.human_required"})
			return
		}
		handler.ServeHTTP(w, r)
	})
	var result http.Handler = guard
	if d.Observability != nil {
		result = d.Observability.Wrap(result)
	}
	return requestIDMiddleware(result)
}

func registerNativeScopeRoutes(mux *http.ServeMux, d Dependencies) {
	for _, pattern := range []string{"GET /api/v1/mindcreek/agent/scope", "POST /api/v1/mindcreek/agent/scope/resolve"} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if d.NativeScopes == nil {
				nativeaccess.WriteError(w, r, &nativeaccess.Error{Status: 503, Code: "scope.unavailable"})
				return
			}
			actor, err := nativeaccess.ResolveActor(r.Context(), d.Principals, r.Header)
			if err != nil {
				nativeaccess.WriteError(w, r, err)
				return
			}
			var input struct {
				Selection string   `json:"selection"`
				IDs       []string `json:"knowledge_base_ids"`
				AgentID   string   `json:"agent_id"`
			}
			input.Selection = "default"
			if r.Method == "POST" {
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
				decoder.DisallowUnknownFields()
				var extra any
				if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF || (input.Selection != "default" && input.Selection != "explicit") {
					nativeaccess.WriteError(w, r, &nativeaccess.Error{Status: 400, Code: "scope.invalid_request"})
					return
				}
			}
			var ids []string
			if input.Selection == "default" && input.AgentID == "" && len(input.IDs) == 0 {
				ids, err = d.NativeScopes.Defaults(r.Context(), r.Header)
			} else {
				ids, err = d.NativeScopes.Resolve(r.Context(), input.IDs, input.Selection == "explicit", input.AgentID, actor, r.Header)
			}
			if err != nil {
				nativeaccess.WriteError(w, r, err)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, map[string]any{"success": true, "data": map[string]any{"knowledge_base_ids": ids, "selection": input.Selection, "authorization": "native", "max_knowledge_bases": nativeaccess.MaxScope}})
		})
	}
}
