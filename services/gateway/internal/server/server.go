package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/capability"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/note"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/policy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/space"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type PrincipalResolver interface {
	CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error)
}

type Dependencies struct {
	Principals PrincipalResolver
	Access     *access.Gate
	Spaces     KnowledgeSpaceService
	Notes      NoteService
	Ingestions IngestionService
}

type IngestionService interface {
	Upload(context.Context, string, string, int64, io.Reader, access.Identity, http.Header) (weknora.Knowledge, error)
	List(context.Context, string, int, int, access.Identity, http.Header) (weknora.KnowledgePage, error)
	Get(context.Context, string, string, access.Identity, http.Header) (weknora.Knowledge, error)
	Retry(context.Context, string, string, access.Identity, http.Header) (weknora.Knowledge, error)
	Cancel(context.Context, string, string, access.Identity, http.Header) (weknora.Knowledge, error)
}

type NoteService interface {
	List(context.Context, string, int, int, access.Identity, http.Header) (note.Page, error)
	Get(context.Context, string, string, access.Identity, http.Header) (note.Note, error)
	Create(context.Context, string, note.WriteInput, access.Identity, http.Header) (note.Note, error)
	Import(context.Context, string, string, []byte, access.Identity, http.Header) (note.Note, error)
	Update(context.Context, string, string, note.WriteInput, access.Identity, http.Header) (note.Note, error)
	Delete(context.Context, string, string, access.Identity, http.Header) error
	ListRevisions(context.Context, string, string, access.Identity) ([]note.Revision, error)
	GetRevision(context.Context, string, string, int, access.Identity) (note.Revision, error)
	Restore(context.Context, string, string, note.RestoreInput, access.Identity, http.Header) (note.Note, error)
}

type KnowledgeSpaceService interface {
	Create(context.Context, space.CreateInput, string, access.Identity, http.Header) (space.CreateResult, error)
	GetProfile(context.Context, string, access.Identity, http.Header) (profile.Profile, error)
}

// NewSkeleton returns the P1-02 transport shell without business behavior.
func NewSkeleton(cfg config.Config) http.Handler {
	return newHandler(cfg, nil, Dependencies{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apierror.Write(w, http.StatusNotFound, "route.not_found", "Route not found", requestID(r))
	}))
}

// NewGateway returns the P1-04 gateway transport. Route policy is added in P1-06.
func NewGateway(cfg config.Config, capabilities *capability.Registry, routePolicy *policy.Policy, dependencies Dependencies) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(cfg.UpstreamURL)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = cfg.UpstreamURL.Host
		r.Header.Set("X-Request-ID", requestID(r))
		for _, header := range []string{"X-MindCreek-User-ID", "X-MindCreek-Owner-ID", "X-MindCreek-Workspace-ID"} {
			r.Header.Del(header)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
		apierror.Write(w, http.StatusBadGateway, "upstream.unavailable", "Upstream service is unavailable", requestID(r))
	}
	if dependencies.Access != nil {
		proxy.ModifyResponse = dependencies.Access.FilterResponse
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			var accessError *access.Error
			if errors.As(err, &accessError) {
				apierror.Write(w, accessError.StatusCode, accessError.Code, accessError.Message, requestID(r))
				return
			}
			apierror.Write(w, http.StatusBadGateway, "upstream.unavailable", "Upstream service is unavailable", requestID(r))
		}
	}
	fallback := http.Handler(proxy)
	if routePolicy != nil {
		fallback = policyHandler(routePolicy, proxy, dependencies)
	}
	return newHandler(cfg, capabilities, dependencies, fallback)
}

func newHandler(cfg config.Config, capabilities *capability.Registry, dependencies Dependencies, fallback http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "mindcreek-gateway",
			"status":  "ok",
		})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service":                    "mindcreek-gateway",
			"version":                    cfg.ProductVersion,
			"compatible_weknora_version": cfg.UpstreamVersion,
		})
	})
	if capabilities != nil {
		mux.HandleFunc("GET /api/v1/capabilities/knowledge-modes", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, capabilities.Document(cfg.ProductVersion, cfg.UpstreamVersion))
		})
	}
	registerNoteRoutes(mux, dependencies)
	registerIngestionRoutes(mux, dependencies)
	mux.HandleFunc("POST /api/v1/knowledge-spaces", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Spaces == nil {
			apierror.Write(w, http.StatusServiceUnavailable, "space.unavailable", "Knowledge-space service is unavailable", requestID(r))
			return
		}
		var input space.CreateInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body is not valid", requestID(r))
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			apierror.Write(w, http.StatusBadRequest, "request.invalid_json", "Request body must contain one JSON document", requestID(r))
			return
		}
		identity := access.Identity{UserID: principal.User.ID, TenantID: principal.Tenant.ID}
		result, err := dependencies.Spaces.Create(r.Context(), input, r.Header.Get("Idempotency-Key"), identity, r.Header)
		if err != nil {
			writeSpaceError(w, r, err)
			return
		}
		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}
		writeJSON(w, status, map[string]any{"success": true, "data": result})
	})
	mux.HandleFunc("GET /api/v1/knowledge-bases/{kb_id}/product-profile", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := resolvePrincipal(w, r, dependencies.Principals)
		if !ok {
			return
		}
		if dependencies.Spaces == nil {
			apierror.Write(w, http.StatusServiceUnavailable, "space.unavailable", "Knowledge-space service is unavailable", requestID(r))
			return
		}
		identity := access.Identity{UserID: principal.User.ID, TenantID: principal.Tenant.ID}
		result, err := dependencies.Spaces.GetProfile(r.Context(), r.PathValue("kb_id"), identity, r.Header)
		if err != nil {
			writeSpaceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
	})
	mux.Handle("/", fallback)
	return requestIDMiddleware(mux)
}

func writeSpaceError(w http.ResponseWriter, r *http.Request, err error) {
	var productError *space.Error
	if errors.As(err, &productError) {
		apierror.Write(w, productError.StatusCode, productError.Code, productError.Message, requestID(r))
		return
	}
	apierror.Write(w, http.StatusInternalServerError, "space.operation_failed", "Knowledge-space operation failed", requestID(r))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type requestIDKey struct{}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 128 {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err == nil {
				id = hex.EncodeToString(raw[:])
			} else {
				id = "unavailable"
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

type principalKey struct{}

func principalFromRequest(r *http.Request) (weknora.Principal, bool) {
	principal, ok := r.Context().Value(principalKey{}).(weknora.Principal)
	return principal, ok
}

func policyHandler(routePolicy *policy.Policy, upstream http.Handler, dependencies Dependencies) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath, err := policy.NormalizeRequestPath(r)
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, "request.path_invalid", "Request path is invalid", requestID(r))
			return
		}
		decision, matched := routePolicy.Match(requestPath)
		if !matched {
			apierror.Write(w, http.StatusNotFound, "route.unclassified", "Route is not available in this MindCreek release", requestID(r))
			return
		}
		if decision.Classification == policy.Disabled {
			apierror.Write(w, http.StatusNotFound, "feature.disabled", "This feature is disabled in MindCreek Phase 1", requestID(r))
			return
		}
		if decision.Classification == policy.KBPolicyControlled {
			principal, ok := resolvePrincipal(w, r, dependencies.Principals)
			if !ok {
				return
			}
			if dependencies.Access == nil {
				apierror.Write(w, http.StatusServiceUnavailable, "security.unavailable", "Authorization service is unavailable", requestID(r))
				return
			}
			identity := access.Identity{UserID: principal.User.ID, TenantID: principal.Tenant.ID}
			if err := dependencies.Access.AuthorizeRequest(r.Context(), r, identity); err != nil {
				writeAccessError(w, r, err)
				return
			}
			w.Header().Set("X-MindCreek-Route-Class", string(policy.KBPolicyControlled))
			ctx := context.WithValue(r.Context(), principalKey{}, principal)
			ctx = access.WithIdentity(ctx, identity)
			r = r.WithContext(ctx)
		}
		upstream.ServeHTTP(w, r)
	})
}

func writeAccessError(w http.ResponseWriter, r *http.Request, err error) {
	var accessError *access.Error
	if errors.As(err, &accessError) {
		apierror.Write(w, accessError.StatusCode, accessError.Code, accessError.Message, requestID(r))
		return
	}
	apierror.Write(w, http.StatusInternalServerError, "security.authorization_failed", "Authorization failed", requestID(r))
}

func resolvePrincipal(w http.ResponseWriter, r *http.Request, resolver PrincipalResolver) (weknora.Principal, bool) {
	if resolver == nil {
		apierror.Write(w, http.StatusServiceUnavailable, "security.unavailable", "Authorization service is unavailable", requestID(r))
		return weknora.Principal{}, false
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" && strings.TrimSpace(r.Header.Get("X-API-Key")) == "" {
		apierror.Write(w, http.StatusUnauthorized, "auth.required", "Authentication is required", requestID(r))
		return weknora.Principal{}, false
	}
	principal, err := resolver.CurrentPrincipal(r.Context(), r.Header)
	if err != nil {
		writePrincipalError(w, r, err)
		return weknora.Principal{}, false
	}
	if principal.User == nil || principal.User.ID == "" || principal.Tenant == nil || principal.Tenant.ID == 0 {
		apierror.Write(w, http.StatusBadGateway, "auth.principal_invalid", "Identity provider returned an invalid principal", requestID(r))
		return weknora.Principal{}, false
	}
	if requestedTenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID")); requestedTenant != "" {
		tenantID, parseErr := strconv.ParseUint(requestedTenant, 10, 64)
		if parseErr != nil || tenantID != principal.Tenant.ID {
			apierror.Write(w, http.StatusForbidden, "workspace.denied", "Requested workspace does not match the authenticated principal", requestID(r))
			return weknora.Principal{}, false
		}
	}
	return principal, true
}

func writePrincipalError(w http.ResponseWriter, r *http.Request, err error) {
	var upstreamError *weknora.Error
	if !errors.As(err, &upstreamError) {
		apierror.Write(w, http.StatusBadGateway, "auth.resolution_failed", "Unable to resolve authenticated principal", requestID(r))
		return
	}
	switch upstreamError.Code {
	case "upstream.unauthorized":
		apierror.Write(w, http.StatusUnauthorized, "auth.invalid", "Authentication is invalid or expired", requestID(r))
	case "upstream.forbidden":
		apierror.Write(w, http.StatusForbidden, "workspace.denied", "Authenticated principal cannot access this workspace", requestID(r))
	default:
		apierror.Write(w, http.StatusBadGateway, "auth.resolution_failed", "Unable to resolve authenticated principal", requestID(r))
	}
}
