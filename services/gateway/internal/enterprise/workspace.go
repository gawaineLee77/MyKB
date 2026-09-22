package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type WorkspaceInput struct {
	PasswordFile, Name, Description, MemberKeyFile string
	AdoptTenantID                                  uint64
	AdoptMemberKeyID                               uint64
}

func tenantPath(id uint64) string { return "/api/v1/tenants/" + strconv.FormatUint(id, 10) }
func tenantHeaders(token string, id uint64) http.Header {
	h := Bearer(token)
	h.Set("X-Tenant-ID", strconv.FormatUint(id, 10))
	return h
}
func keyHeaders(token string, id uint64) http.Header {
	h := http.Header{}
	h.Set("X-API-Key", token)
	h.Set("X-Tenant-ID", strconv.FormatUint(id, 10))
	return h
}

func WriteSecret(path, value string) error {
	if path == "" || value == "" || !filepath.IsAbs(path) {
		return failure("install.secret_invalid", 503)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || !info.IsDir() {
		return failure("install.secret_directory_missing", 503)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return failure("install.secret_already_exists", 409)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".member-key-*")
	if err != nil {
		return failure("install.secret_write_failed", 503)
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return failure("install.secret_write_failed", 503)
	}
	// Hard-link installation refuses to overwrite a secret another process made.
	if err = os.Link(f.Name(), path); err != nil {
		return failure("install.secret_write_failed", 503)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return failure("install.secret_write_failed", 503)
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return failure("install.secret_write_failed", 503)
	}
	return nil
}

func memberRole(p weknora.Principal, id uint64) string {
	for _, m := range p.Memberships {
		if m.TenantID == id {
			return m.Role
		}
	}
	return ""
}

func (s *Service) DefaultWorkspace(ctx context.Context, input WorkspaceInput) (Installation, error) {
	var result Installation
	err := s.Store.WithLock(ctx, "enterprise.install", func(store Store) error {
		i, err := store.Installation(ctx)
		if err != nil {
			return err
		}
		result = i
		if i.Stage == "ready" {
			return nil
		}
		if i.Stage != "account_ready" && i.Stage != "space_creating" && i.Stage != "space_ready" && i.Stage != "key_creating" {
			return failure("install.account_not_ready", 409)
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
		principal, err := s.Upstream.CurrentPrincipal(ctx, Bearer(token.Token))
		if err != nil || !i.IsAdmin(principal) {
			return failure("admin.required", 403)
		}
		if i.Stage == "account_ready" || i.Stage == "space_creating" {
			if input.AdoptTenantID != 0 {
				if i.Stage != "space_creating" || memberRole(principal, input.AdoptTenantID) != "owner" {
					return failure("install.space_adoption_denied", 409)
				}
				i.DefaultTenantID = input.AdoptTenantID
			} else {
				if i.Stage == "space_creating" {
					return failure("install.space_result_unknown", 409)
				}
				if input.Name == "" {
					return failure("install.space_name_required", 400)
				}
				i.Stage = "space_creating"
				if err := store.SaveInstallation(ctx, i); err != nil {
					return err
				}
				raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/tenants", Bearer(token.Token), map[string]string{"name": input.Name, "description": input.Description})
				if err != nil {
					return failure("install.space_result_unknown", 409)
				}
				var response struct {
					Success bool `json:"success"`
					Data    struct {
						ID uint64 `json:"id"`
					} `json:"data"`
				}
				if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.ID == 0 {
					return failure("install.space_result_unknown", 409)
				}
				i.DefaultTenantID = response.Data.ID
			}
			i.Stage = "space_ready"
			if err := store.SaveInstallation(ctx, i); err != nil {
				return err
			}
		}
		headers := tenantHeaders(token.Token, i.DefaultTenantID)
		principal, err = s.Upstream.CurrentPrincipal(ctx, headers)
		if err != nil || memberRole(principal, i.DefaultTenantID) != "owner" {
			return failure("install.owner_required", 403)
		}
		for _, setting := range []struct {
			key   string
			value any
		}{
			{"auth.registration_mode", "invite_only"}, {"auth.default_tenant_mode", "tenantless"}, {"tenant.self_service_creation_enabled", true},
		} {
			if _, err := s.Upstream.EnterpriseRequest(ctx, "PUT", "/api/v1/system/admin/settings/"+setting.key, headers, map[string]any{"value": setting.value}); err != nil {
				return err
			}
		}
		if input.AdoptMemberKeyID != 0 {
			if i.Stage != "key_creating" {
				return failure("install.key_recovery_not_required", 409)
			}
			raw, err := s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(i.DefaultTenantID)+"/api-keys", headers, nil)
			if err != nil {
				return err
			}
			var response struct {
				Data []struct {
					ID           uint64   `json:"id"`
					Key          string   `json:"api_key"`
					Full         bool     `json:"full_access"`
					Capabilities []string `json:"capabilities"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &response) != nil {
				return failure("upstream.invalid_response", 502)
			}
			secret, err := ReadSecret(input.MemberKeyFile)
			if err != nil {
				return err
			}
			valid := false
			for _, k := range response.Data {
				if k.ID == input.AdoptMemberKeyID && k.Key == secret && !k.Full && len(k.Capabilities) == 1 && k.Capabilities[0] == "manage_members" {
					valid = true
				}
			}
			if !valid {
				return failure("install.key_scope_invalid", 409)
			}
			i.MemberKeyID = strconv.FormatUint(input.AdoptMemberKeyID, 10)
			i.MemberKeyRef = input.MemberKeyFile
		} else if i.Stage == "space_ready" {
			if input.MemberKeyFile == "" {
				return failure("install.secret_invalid", 503)
			}
			i.Stage = "key_creating"
			i.MemberKeyRef = input.MemberKeyFile
			if err := store.SaveInstallation(ctx, i); err != nil {
				return err
			}
			raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", tenantPath(i.DefaultTenantID)+"/api-keys", headers, map[string]any{"name": "MindCreek employee onboarding", "full_access": false, "capabilities": []string{"manage_members"}})
			if err != nil {
				return failure("install.key_result_unknown", 409)
			}
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					ID    uint64 `json:"id"`
					Token string `json:"token"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.ID == 0 || response.Data.Token == "" {
				return failure("install.key_result_unknown", 409)
			}
			i.MemberKeyID = strconv.FormatUint(response.Data.ID, 10)
			if err := store.SaveInstallation(ctx, i); err != nil {
				return err
			}
			if err := WriteSecret(i.MemberKeyRef, response.Data.Token); err != nil {
				return err
			}
		} else if i.MemberKeyID == "" {
			return failure("install.key_result_unknown", 409)
		}
		secret, err := ReadSecret(i.MemberKeyRef)
		if err != nil {
			return err
		}
		if _, err = s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(i.DefaultTenantID)+"/members", keyHeaders(secret, i.DefaultTenantID), nil); err != nil {
			return failure("install.member_key_unavailable", 503)
		}
		i.Stage = "ready"
		i.ErrorCode = ""
		if err := store.SaveInstallation(ctx, i); err != nil {
			return err
		}
		result = i
		return nil
	})
	return result, err
}
