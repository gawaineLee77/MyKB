package nativeaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/apierror"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/policy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type Actor struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	TenantID uint64 `json:"tenant_id"`
}

type PrincipalResolver interface {
	CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error)
}
type RequestCheck func(context.Context, *http.Request, Actor) error

type Gate struct {
	Routes        *Routes
	Principals    PrincipalResolver
	ScopeCheck    RequestCheck
	ModelCheck    RequestCheck
	ResponseCheck func(*http.Response) error
	LocalRequest  func(http.ResponseWriter, *http.Request, Actor) (bool, error)
}

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string { return e.Code }

func ResolveActor(ctx context.Context, resolver PrincipalResolver, headers http.Header) (Actor, error) {
	if resolver == nil {
		return Actor{}, &Error{503, "identity.unavailable"}
	}
	bearer, key := strings.TrimSpace(headers.Get("Authorization")), strings.TrimSpace(headers.Get("X-API-Key"))
	if bearer == "" && key == "" {
		return Actor{}, &Error{401, "auth.required"}
	}
	if bearer != "" && key != "" {
		return Actor{}, &Error{400, "auth.ambiguous_credentials"}
	}
	p, err := resolver.CurrentPrincipal(ctx, headers)
	if err != nil {
		return Actor{}, err
	}
	if p.Tenant == nil || p.Tenant.ID == 0 {
		return Actor{}, &Error{409, "workspace.required"}
	}
	if requested := headers.Get("X-Tenant-ID"); requested != "" && requested != strconv.FormatUint(p.Tenant.ID, 10) {
		return Actor{}, &Error{403, "workspace.denied"}
	}
	if key != "" {
		// The native auth/me user is not an API key's human identity. Fingerprints
		// separate credentials without persisting their reusable secret material.
		digest := sha256.Sum256([]byte("mindcreek:r3:api-key:" + key))
		return Actor{Kind: "api_key", ID: hex.EncodeToString(digest[:]), TenantID: p.Tenant.ID}, nil
	}
	if p.User == nil || p.User.ID == "" || !p.User.IsActive {
		return Actor{}, &Error{403, "identity.inactive"}
	}
	return Actor{Kind: "human", ID: p.User.ID, TenantID: p.Tenant.ID}, nil
}

func (g *Gate) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if body, ok := r.Body.(*uploadBody); ok {
				_ = body.Close()
			}
		}()
		path, err := policy.NormalizeRequestPath(r)
		if err != nil {
			WriteError(w, r, &Error{400, "request.path_invalid"})
			return
		}
		if Retired(path) {
			WriteError(w, r, &Error{410, "feature.retired"})
			return
		}
		route, ok := g.Routes.Match(r.Method, path)
		if !ok {
			WriteError(w, r, &Error{404, "route.unclassified"})
			return
		}
		if route.Disposition == "disabled" {
			WriteError(w, r, &Error{404, "feature.disabled"})
			return
		}
		// Authentication/recovery remains owned by R2 and the native handler.
		if strings.HasPrefix(path, "/api/v1/auth/") || path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		needsActor := false
		for _, name := range route.Checks {
			if name != "enterprise" {
				needsActor = true
			}
		}
		if !needsActor {
			next.ServeHTTP(w, r)
			return
		}
		actor, err := ResolveActor(r.Context(), g.Principals, r.Header)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		for _, name := range route.Checks {
			var check RequestCheck
			switch name {
			case "scope_session", "agent_binding":
				check = g.ScopeCheck
			case "models_upload", "feature_settings":
				check = g.ModelCheck
			default:
				continue
			}
			if check == nil {
				WriteError(w, r, &Error{503, "native.adapter_not_ready"})
				return
			}
			if err := check(r.Context(), r, actor); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		if g.LocalRequest != nil {
			handled, err := g.LocalRequest(w, r, actor)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if handled {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Gate) FilterResponse(r *http.Response) error {
	if g.ResponseCheck != nil {
		return g.ResponseCheck(r)
	}
	return nil
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 503, "native.authorization_unavailable"
	var nativeErr *Error
	var upstreamErr *weknora.Error
	switch {
	case errors.As(err, &nativeErr):
		status, code = nativeErr.Status, nativeErr.Code
	case errors.As(err, &upstreamErr):
		status, code = upstreamErr.StatusCode, upstreamErr.Code
	}
	apierror.Write(w, status, code, code, r.Header.Get("X-Request-ID"))
}
