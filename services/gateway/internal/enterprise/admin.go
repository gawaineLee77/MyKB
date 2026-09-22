package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Service) AdminAuth(ctx context.Context, action string, h http.Header, input json.RawMessage) (json.RawMessage, error) {
	i, err := s.Store.Installation(ctx)
	if err != nil {
		return nil, failure("install.not_initialized", 503)
	}
	if i.AdminUserID == "" {
		return nil, failure("install.admin_not_ready", 503)
	}
	if action == "login" {
		var credentials struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if json.Unmarshal(input, &credentials) != nil || !strings.EqualFold(strings.TrimSpace(credentials.Email), i.AdminEmail) || credentials.Password == "" {
			return nil, failure("admin.authentication_failed", 401)
		}
	} else if action == "refresh" {
		var body struct {
			Token string `json:"refreshToken"`
		}
		if json.Unmarshal(input, &body) != nil || body.Token == "" {
			return nil, failure("admin.authentication_failed", 401)
		}
	} else if action == "logout" || action == "change-password" {
		p, err := s.Upstream.CurrentPrincipal(ctx, identityHeaders(h))
		if err != nil || !i.IsAdmin(p) {
			return nil, failure("admin.authentication_failed", 401)
		}
	} else {
		return nil, failure("admin.action_invalid", 404)
	}
	if action == "login" {
		raw, p, err := s.checkedPasswordLogin(ctx, input)
		if err != nil {
			return nil, failure("admin.authentication_failed", 401)
		}
		if !i.IsAdmin(p) {
			var result session
			_ = json.Unmarshal(raw, &result)
			_, _ = s.Upstream.EnterpriseRequest(context.WithoutCancel(ctx), "POST", "/api/v1/auth/logout", Bearer(result.Token), nil)
			return nil, failure("admin.authentication_failed", 401)
		}
		return raw, nil
	}
	headers := identityHeaders(h)
	if action == "login" || action == "refresh" {
		headers = nil
	}
	raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/"+action, headers, input)
	if err != nil {
		return nil, err
	}
	if action == "login" || action == "refresh" {
		var result session
		if json.Unmarshal(raw, &result) != nil || !result.Success {
			return nil, failure("upstream.invalid_response", 502)
		}
		token := result.Token
		if action == "refresh" {
			token = result.AccessToken
		}
		if token == "" || result.RefreshToken == "" {
			return nil, failure("upstream.invalid_response", 502)
		}
		p, err := s.Upstream.CurrentPrincipal(ctx, Bearer(token))
		if err != nil || !i.IsAdmin(p) {
			_, _ = s.Upstream.EnterpriseRequest(context.WithoutCancel(ctx), "POST", "/api/v1/auth/logout", Bearer(token), nil)
			return nil, failure("admin.authentication_failed", 401)
		}
	}
	return raw, nil
}

// Identity operations must also work after the last membership is removed.
// Never forward a stale workspace or a second credential to these endpoints.
func identityHeaders(h http.Header) http.Header {
	result := http.Header{}
	for _, key := range []string{"Authorization", "X-Request-ID", "Accept-Language"} {
		if value := h.Get(key); value != "" {
			result.Set(key, value)
		}
	}
	return result
}
