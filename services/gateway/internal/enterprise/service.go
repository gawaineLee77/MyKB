package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type Upstream interface {
	EnterpriseRequest(context.Context, string, string, http.Header, any) (json.RawMessage, error)
	CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error)
}

type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string              { return e.Code }
func failure(code string, status int) error { return &Error{Code: code, Status: status} }

// Never expose database errors, upstream response bodies or secret file paths.
func PublicError(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if errors.Is(err, ErrNotFound) {
		return "install.not_initialized"
	}
	return "enterprise.operation_failed"
}

type Service struct {
	Store      Store
	Upstream   Upstream
	Identities IdentityStore
}

type IdentityStore interface {
	GetByUpstreamEmail(context.Context, string) (identity.Identity, error)
	BindLocalAccount(context.Context, string, string) error
	MemberAlias(context.Context, string) (string, error)
}

type InstallInput struct{ Email, Username, PasswordFile, AdoptAdminID string }

type session struct {
	Success      bool          `json:"success"`
	Token        string        `json:"token"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	User         *weknora.User `json:"user"`
}

func Bearer(token string) http.Header {
	return http.Header{"Authorization": []string{"Bearer " + token}}
}

func ReadSecret(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return "", failure("install.secret_invalid", 503)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", failure("install.secret_unavailable", 503)
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if value == "" {
		return "", failure("install.secret_empty", 503)
	}
	return value, nil
}

func (s *Service) login(ctx context.Context, email, password string) (session, error) {
	raw, _, err := s.checkedPasswordLogin(ctx, map[string]string{"email": email, "password": password})
	if err != nil {
		return session{}, failure("admin.authentication_failed", 401)
	}
	var result session
	if json.Unmarshal(raw, &result) != nil || !result.Success || result.Token == "" || result.RefreshToken == "" {
		return result, failure("upstream.invalid_response", 502)
	}
	return result, nil
}

// Prepare creates only the configured account. A persisted 'registering'
// marker requires explicit recovery after an ambiguous upstream response.
func (s *Service) Prepare(ctx context.Context, input InstallInput) (Installation, error) {
	var result Installation
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if input.Email == "" || !strings.Contains(input.Email, "@") || strings.TrimSpace(input.Username) == "" {
		return result, failure("install.config_missing", 400)
	}
	err := s.Store.WithLock(ctx, "enterprise.install", func(store Store) error {
		i, err := store.Installation(ctx)
		if errors.Is(err, ErrNotFound) {
			i = Installation{Stage: "new", AdminEmail: input.Email}
		} else if err != nil {
			return err
		}
		result = i
		if i.AdminEmail != input.Email {
			return failure("install.account_conflict", 409)
		}
		if i.Stage != "new" && i.Stage != "registering" {
			return nil
		}
		password, err := ReadSecret(input.PasswordFile)
		if err != nil {
			return err
		}
		if i.Stage == "registering" && input.AdoptAdminID == "" {
			return failure("install.account_result_unknown", 409)
		}
		if input.AdoptAdminID != "" {
			if i.Stage != "registering" {
				return failure("install.recovery_not_required", 409)
			}
			token, err := s.login(ctx, input.Email, password)
			if err != nil {
				return err
			}
			defer s.Upstream.EnterpriseRequest(context.WithoutCancel(ctx), "POST", "/api/v1/auth/logout", Bearer(token.Token), nil)
			p, err := s.Upstream.CurrentPrincipal(ctx, Bearer(token.Token))
			if err != nil || p.User == nil || p.User.ID != input.AdoptAdminID || !strings.EqualFold(p.User.Email, input.Email) || p.Tenant != nil || len(p.Memberships) != 0 {
				return failure("install.account_conflict", 409)
			}
			i.AdminUserID = p.User.ID
		} else {
			i.Stage = "registering"
			if err := store.SaveInstallation(ctx, i); err != nil {
				return err
			}
			raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/register", nil, map[string]string{"email": input.Email, "username": input.Username, "password": password})
			if err != nil {
				i.ErrorCode = "install.account_result_unknown"
				_ = store.SaveInstallation(ctx, i)
				return failure(i.ErrorCode, 409)
			}
			var registered session
			if json.Unmarshal(raw, &registered) != nil || !registered.Success || registered.User == nil || registered.User.ID == "" || registered.User.TenantID != 0 {
				return failure("install.tenantless_required", 409)
			}
			i.AdminUserID = registered.User.ID
		}
		i.Stage = "admin_registered"
		i.ErrorCode = ""
		if err := store.SaveInstallation(ctx, i); err != nil {
			return err
		}
		result = i
		return nil
	})
	return result, err
}

// ConfirmAdmin runs after the operator-controlled bootstrap restart. No
// product SQL writes or API calls grant platform privileges.
func (s *Service) ConfirmAdmin(ctx context.Context, input InstallInput) (Installation, error) {
	var result Installation
	err := s.Store.WithLock(ctx, "enterprise.install", func(store Store) error {
		i, err := store.Installation(ctx)
		if err != nil {
			return err
		}
		result = i
		if !strings.EqualFold(i.AdminEmail, strings.TrimSpace(input.Email)) {
			return failure("install.account_conflict", 409)
		}
		if i.Stage == "ready" || i.Stage == "account_ready" || i.Stage == "space_ready" {
			return nil
		}
		if i.Stage != "admin_registered" {
			return failure("install.stage_invalid", 409)
		}
		password, err := ReadSecret(input.PasswordFile)
		if err != nil {
			return err
		}
		token, err := s.login(ctx, i.AdminEmail, password)
		if err != nil {
			return err
		}
		defer s.Upstream.EnterpriseRequest(context.WithoutCancel(ctx), "POST", "/api/v1/auth/logout", Bearer(token.Token), nil)
		p, err := s.Upstream.CurrentPrincipal(ctx, Bearer(token.Token))
		if err != nil || !i.IsAdmin(p) {
			return failure("install.bootstrap_required_or_conflict", 409)
		}
		// v0.8.0 requires a tenant for human system-settings requests. The
		// deployment closes registration before confirmation; persist the DB
		// policy only after default-space creation. An empty invalid request
		// cannot create an account and distinguishes closed (403) from open (400).
		_, err = s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/register", nil, map[string]string{})
		var upstreamError *weknora.Error
		if !errors.As(err, &upstreamError) || upstreamError.UpstreamStatus != 403 {
			return failure("install.registration_must_be_closed", 409)
		}
		i.Stage = "account_ready"
		i.ErrorCode = ""
		if err := store.SaveInstallation(ctx, i); err != nil {
			return err
		}
		result = i
		return nil
	})
	return result, err
}

func (i Installation) IsAdmin(p weknora.Principal) bool {
	return i.AdminUserID != "" && p.User != nil && p.User.ID == i.AdminUserID && p.User.IsSystemAdmin && p.User.IsActive
}
