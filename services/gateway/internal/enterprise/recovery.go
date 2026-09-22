package enterprise

import (
	"context"
	"encoding/json"
	"strconv"
)

// RepairMemberKey adopts a current Owner's explicitly supplied scoped key.
// It neither creates a space nor changes membership or platform ownership.
func (s *Service) RepairMemberKey(ctx context.Context, ownerBearerFile, keyFile string, keyID uint64) (Installation, error) {
	var result Installation
	err := s.Store.WithLock(ctx, "enterprise.install", func(store Store) error {
		i, err := store.Installation(ctx)
		if err != nil {
			return err
		}
		if i.Stage != "ready" || keyID == 0 {
			return failure("install.key_recovery_not_ready", 409)
		}
		bearer, err := ReadSecret(ownerBearerFile)
		if err != nil {
			return err
		}
		h := tenantHeaders(bearer, i.DefaultTenantID)
		p, err := s.Upstream.CurrentPrincipal(ctx, h)
		if err != nil || p.User == nil || !p.User.IsActive || memberRole(p, i.DefaultTenantID) != "owner" {
			return failure("install.owner_required", 403)
		}
		key, err := ReadSecret(keyFile)
		if err != nil {
			return err
		}
		raw, err := s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(i.DefaultTenantID)+"/api-keys", h, nil)
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
		valid := false
		for _, k := range response.Data {
			if k.ID == keyID && k.Key == key && !k.Full && len(k.Capabilities) == 1 && k.Capabilities[0] == "manage_members" {
				valid = true
			}
		}
		if !valid {
			return failure("install.key_scope_invalid", 409)
		}
		if _, err := s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(i.DefaultTenantID)+"/members", keyHeaders(key, i.DefaultTenantID), nil); err != nil {
			return failure("install.member_key_unavailable", 503)
		}
		i.MemberKeyID, i.MemberKeyRef, i.ErrorCode = strconv.FormatUint(keyID, 10), keyFile, ""
		if err := store.SaveInstallation(ctx, i); err != nil {
			return err
		}
		result = i
		return nil
	})
	return result, err
}
