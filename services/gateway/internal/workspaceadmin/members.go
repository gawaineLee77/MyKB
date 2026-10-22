package workspaceadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

// Manage extends the human Admin role through the product boundary. Every
// Owner and Admin mutation uses the same distributed workspace lock, so an
// Owner promotion cannot race the Admin's target-role check. Upstream must
// remain private: direct upstream mutations cannot take this product lock.
func (s *Service) Manage(ctx context.Context, p weknora.Principal, h http.Header, tenantID uint64, method, suffix string, raw json.RawMessage) (json.RawMessage, error) {
	operation, input, target, err := memberOperation(method, suffix, raw)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	err = s.withSpace(ctx, tenantID, func(store Store) error {
		current, err := s.freshPrincipal(ctx, p, h, tenantID)
		if err != nil {
			return err
		}
		callerRole := role(current, tenantID)
		if callerRole != "owner" && callerRole != "admin" {
			return failure("workspace.admin_required", http.StatusForbidden)
		}
		var member *weknora.TenantMember
		if strings.HasPrefix(suffix, "/members/") {
			member, err = s.memberTarget(ctx, tenantID, h, target, "")
			if err != nil {
				return err
			}
			if member == nil {
				return failure("workspace.member_not_found", http.StatusNotFound)
			}
		}
		if callerRole == "admin" {
			if assigned, _ := input["role"].(string); assigned == "owner" {
				return failure("workspace.owner_protected", http.StatusForbidden)
			}
			if strings.HasPrefix(suffix, "/members/") {
				if member.Role == "owner" {
					return failure("workspace.owner_protected", http.StatusForbidden)
				}
			}
			if method == http.MethodPost && (suffix == "/members" || suffix == "/invitations") {
				member, err := s.memberTarget(ctx, tenantID, h, "", input["email"].(string))
				if err != nil {
					return err
				}
				if member != nil && member.Role == "owner" {
					return failure("workspace.owner_protected", http.StatusForbidden)
				}
			}
			if strings.HasPrefix(suffix, "/invitations/") {
				if err := s.checkInvitationTarget(ctx, tenantID, h, target); err != nil {
					return err
				}
			}
		}
		// Persist attribution before the upstream side effect. The native audit
		// sees the delegated credential; this record identifies the human actor.
		if err := s.audit(ctx, store, tenantID, current.User.ID, operation, target, "attempt"); err != nil {
			return err
		}
		newRole, _ := input["role"].(string)
		disableKeys := member != nil && (method == http.MethodDelete || memberRoleRank(newRole) < memberRoleRank(member.Role))
		if disableKeys {
			if member.Role == "owner" {
				count, err := s.activeOwnerCount(ctx, tenantID, h)
				if err != nil {
					return err
				}
				if count <= 1 {
					return failure("workspace.last_owner", http.StatusConflict)
				}
			}
		}
		forward := h
		if callerRole == "admin" {
			if suffix == "" {
				forward, err = s.ownerHeaders(ctx, store, current, h, tenantID)
			} else {
				forward, err = s.memberHeaders(ctx, store, current, h, tenantID)
			}
			if err != nil {
				return err
			}
		}
		if disableKeys {
			// This durable tombstone precedes the upstream mutation so a later
			// promotion cannot revive a previously revoked machine credential.
			if err := store.DisableUserKeys(ctx, tenantID, target); err != nil {
				return err
			}
		}
		if s.RevokeKnowledgeMember != nil && member != nil && (method == http.MethodDelete || newRole == "viewer") {
			if err := s.RevokeKnowledgeMember(ctx, tenantID, target, method == http.MethodDelete, current.User.ID); err != nil {
				return err
			}
		}
		result, err = s.Upstream.EnterpriseRequest(ctx, method, tenantPath(tenantID)+suffix, forward, input)
		outcome := "success"
		if err != nil {
			outcome = "failure"
		}
		auditErr := s.audit(ctx, store, tenantID, current.User.ID, operation, target, outcome)
		if err != nil {
			if disableKeys {
				_ = s.audit(ctx, store, tenantID, current.User.ID, "workspace.member.keys_disabled", target, "failure")
				return failure("workspace.member_update_failed_keys_disabled", http.StatusBadGateway)
			}
			return err
		}
		return auditErr
	})
	return result, err
}

// SelfLeave uses the same lock as member administration. In particular, an
// Admin cannot leave while another in-flight action is using that membership.
func (s *Service) SelfLeave(ctx context.Context, p weknora.Principal, h http.Header, tenantID uint64) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.withSpace(ctx, tenantID, func(store Store) error {
		if !strings.HasPrefix(h.Get("Authorization"), "Bearer ") || h.Get("X-API-Key") != "" {
			return failure("workspace_admin.human_required", http.StatusForbidden)
		}
		fresh, err := s.Upstream.CurrentPrincipal(ctx, h)
		if err != nil {
			return failure("workspace_admin.authentication_failed", http.StatusUnauthorized)
		}
		if fresh.User == nil || !fresh.User.IsActive || p.User == nil || fresh.User.ID != p.User.ID || fresh.Tenant == nil || fresh.Tenant.ID != tenantID || memberRoleRank(role(fresh, tenantID)) == 0 {
			return failure("workspace_admin.access_denied", http.StatusForbidden)
		}
		if s.IdentityAllowed == nil {
			return failure("workspace_admin.identity_unavailable", http.StatusServiceUnavailable)
		}
		if err := s.IdentityAllowed(ctx, fresh.User.ID); err != nil {
			return identityFailure(err)
		}
		if role(fresh, tenantID) == "owner" {
			owners, err := s.activeOwnerCount(ctx, tenantID, h)
			if err != nil {
				return err
			}
			if owners <= 1 {
				return failure("workspace.last_owner", http.StatusConflict)
			}
		}
		if err := s.audit(ctx, store, tenantID, fresh.User.ID, "workspace.member.leave", fresh.User.ID, "attempt"); err != nil {
			return err
		}
		if err := store.DisableUserKeys(ctx, tenantID, fresh.User.ID); err != nil {
			return err
		}
		if s.RevokeKnowledgeMember != nil {
			if err := s.RevokeKnowledgeMember(ctx, tenantID, fresh.User.ID, true, fresh.User.ID); err != nil {
				return err
			}
		}
		result, err = s.Upstream.EnterpriseRequest(ctx, http.MethodPost, tenantPath(tenantID)+"/leave", ownerHeader(strings.TrimPrefix(h.Get("Authorization"), "Bearer "), tenantID), nil)
		if err != nil {
			_ = s.audit(ctx, store, tenantID, fresh.User.ID, "workspace.member.leave", fresh.User.ID, "failure_keys_disabled")
			return failure("workspace.member_update_failed_keys_disabled", http.StatusBadGateway)
		}
		return s.audit(ctx, store, tenantID, fresh.User.ID, "workspace.member.leave", fresh.User.ID, "success")
	})
	return result, err
}

// ManageMachine retains native machine-capability authorization while sharing
// the human mutation lock and key-revocation lifecycle. It never exchanges the
// caller's API key for a delegated Owner session.
func (s *Service) ManageMachine(ctx context.Context, h http.Header, tenantID uint64, method, suffix string, raw json.RawMessage) (json.RawMessage, error) {
	if h.Get("X-API-Key") == "" || h.Get("Authorization") != "" {
		return nil, failure("workspace_admin.machine_required", http.StatusForbidden)
	}
	if suffix == "" || suffix == "/leave" {
		return nil, failure("workspace.route_invalid", http.StatusNotFound)
	}
	operation, input, target, err := memberOperation(method, suffix, raw)
	if err != nil {
		return nil, err
	}
	if assigned, _ := input["role"].(string); assigned == "owner" {
		// Native manage_members forbids ownership grants for every machine key.
		return nil, failure("workspace.machine_owner_grant_denied", http.StatusForbidden)
	}
	var result json.RawMessage
	err = s.withSpace(ctx, tenantID, func(store Store) error {
		if err := s.checkKeyUseLocked(ctx, store, h.Get("X-API-Key"), tenantID); err != nil {
			return err
		}
		forward := make(http.Header)
		forward.Set("X-API-Key", h.Get("X-API-Key"))
		for _, name := range []string{"X-External-User-ID", "X-External-User-Token"} {
			if value := h.Get(name); value != "" {
				forward.Set(name, value)
			}
		}
		if active := h.Get("X-Tenant-ID"); active != "" {
			parsed, err := strconv.ParseUint(active, 10, 64)
			if err != nil || parsed != tenantID {
				return failure("workspace_admin.key_tenant_denied", http.StatusForbidden)
			}
			forward.Set("X-Tenant-ID", active)
		}
		// This native route has the same manage_members/full-access gate as
		// member mutations. Authenticate and authorize before touching local
		// creator-key metadata, including for untracked legacy credentials.
		probe, err := s.Upstream.EnterpriseRequest(ctx, http.MethodGet, tenantPath(tenantID)+"/members?page=1&page_size=1", forward, nil)
		if err != nil {
			return err
		}
		var authorized struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(probe, &authorized) != nil || !authorized.Success {
			return failure("upstream.invalid_response", http.StatusBadGateway)
		}
		var member *weknora.TenantMember
		if strings.HasPrefix(suffix, "/members/") {
			member, err = s.memberTarget(ctx, tenantID, forward, target, "")
			if err != nil {
				return err
			}
			if member == nil {
				return failure("workspace.member_not_found", http.StatusNotFound)
			}
		}
		newRole, _ := input["role"].(string)
		disableKeys := member != nil && (method == http.MethodDelete || memberRoleRank(newRole) < memberRoleRank(member.Role))
		if disableKeys && member.Role == "owner" {
			owners, err := s.activeOwnerCount(ctx, tenantID, forward)
			if err != nil {
				return err
			}
			if owners <= 1 {
				return failure("workspace.last_owner", http.StatusConflict)
			}
		}
		actor := "api-key:" + fingerprint(h.Get("X-API-Key"))
		if err := s.audit(ctx, store, tenantID, actor, operation+".machine", target, "attempt"); err != nil {
			return err
		}
		if disableKeys {
			if err := store.DisableUserKeys(ctx, tenantID, target); err != nil {
				return err
			}
		}
		if s.RevokeKnowledgeMember != nil && member != nil && (method == http.MethodDelete || newRole == "viewer") {
			if err := s.RevokeKnowledgeMember(ctx, tenantID, target, method == http.MethodDelete, actor); err != nil {
				return err
			}
		}
		result, err = s.Upstream.EnterpriseRequest(ctx, method, tenantPath(tenantID)+suffix, forward, input)
		if err != nil {
			_ = s.audit(ctx, store, tenantID, actor, operation+".machine", target, "failure")
			if disableKeys {
				return failure("workspace.member_update_failed_keys_disabled", http.StatusBadGateway)
			}
			return err
		}
		return s.audit(ctx, store, tenantID, actor, operation+".machine", target, "success")
	})
	return result, err
}

func memberRoleRank(value string) int {
	switch value {
	case "owner":
		return 4
	case "admin":
		return 3
	case "contributor":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

func (s *Service) activeOwnerCount(ctx context.Context, tenantID uint64, h http.Header) (int, error) {
	seen := map[string]bool{}
	owners := 0
	for page := 1; page <= 500; page++ {
		raw, err := s.Upstream.EnterpriseRequest(ctx, http.MethodGet, fmt.Sprintf("%s/members?page=%d&page_size=100", tenantPath(tenantID), page), h, nil)
		if err != nil {
			return 0, err
		}
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Members []weknora.TenantMember `json:"members"`
				Total   int                    `json:"total"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.Total < 0 {
			return 0, failure("upstream.invalid_response", http.StatusBadGateway)
		}
		for _, member := range response.Data.Members {
			if member.UserID == "" || seen[member.UserID] || memberRoleRank(member.Role) == 0 {
				return 0, failure("upstream.invalid_response", http.StatusBadGateway)
			}
			seen[member.UserID] = true
			if member.Role == "owner" && member.Status == "active" {
				owners++
			}
		}
		if len(seen) >= response.Data.Total {
			return owners, nil
		}
		if len(response.Data.Members) == 0 {
			break
		}
	}
	return 0, failure("workspace.member_lookup_incomplete", http.StatusServiceUnavailable)
}

// memberHeaders provisions a workspace-scoped machine key with exactly the
// manage_members capability. A valid delegated Owner session is required on
// every use, including when the internal key is already cached. Workspaces
// that require an external-user proof use the already-authorized Owner JWT;
// no signing secret or fabricated external identity is needed for this proxy.
func (s *Service) memberHeaders(ctx context.Context, store Store, p weknora.Principal, h http.Header, tenantID uint64) (http.Header, error) {
	owner, err := s.ownerHeaders(ctx, store, p, h, tenantID)
	if err != nil {
		return nil, err
	}
	rawConfig, err := s.Upstream.EnterpriseRequest(ctx, http.MethodGet, tenantPath(tenantID)+"/api-principal-config", owner, nil)
	if err != nil {
		return nil, err
	}
	var config struct {
		Success bool `json:"success"`
		Data    *struct {
			Mode                string `json:"mode"`
			RequireDirectHeader bool   `json:"require_direct_header"`
		} `json:"data"`
	}
	if json.Unmarshal(rawConfig, &config) != nil || !config.Success || config.Data == nil {
		return nil, failure("upstream.invalid_response", http.StatusBadGateway)
	}
	switch config.Data.Mode {
	case "tenant":
	case "direct_header":
		if config.Data.RequireDirectHeader {
			return owner, nil
		}
	case "signed_token":
		return owner, nil
	default:
		return nil, failure("upstream.invalid_response", http.StatusBadGateway)
	}
	delegation, err := store.Delegation(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if delegation.MemberKeyID != 0 && len(delegation.MemberKeyCipher) > 0 {
		secret, err := s.open(tenantID, "member-key", delegation.MemberKeyCipher)
		if err != nil || len(secret) == 0 {
			return nil, failure("workspace.member_credential_unavailable", http.StatusServiceUnavailable)
		}
		return memberKeyHeader(string(secret), tenantID), nil
	}
	const internalName = "mindcreek-internal:workspace-members"
	// A stable reserved name recovers an upstream create whose response/save
	// was interrupted. Reserved internal keys never enter the user key list.
	raw, err := s.Upstream.EnterpriseRequest(ctx, http.MethodGet, tenantPath(tenantID)+"/api-keys", owner, nil)
	if err != nil {
		return nil, err
	}
	var listed struct {
		Success bool     `json:"success"`
		Data    []keyDTO `json:"data"`
	}
	if json.Unmarshal(raw, &listed) != nil || !listed.Success {
		return nil, failure("upstream.invalid_response", http.StatusBadGateway)
	}
	var key keyDTO
	for _, candidate := range listed.Data {
		if candidate.Name == internalName {
			if key.ID != 0 {
				return nil, failure("workspace.member_credential_conflict", http.StatusConflict)
			}
			key = candidate
		}
	}
	if key.ID == 0 {
		requested := keyInput{Name: internalName, Capabilities: []string{"manage_members"}}
		raw, err = s.Upstream.EnterpriseRequest(ctx, http.MethodPost, tenantPath(tenantID)+"/api-keys", owner, requested)
		if err != nil {
			return nil, err
		}
		var created struct {
			Success bool   `json:"success"`
			Data    keyDTO `json:"data"`
		}
		if json.Unmarshal(raw, &created) != nil || !created.Success {
			return nil, failure("upstream.invalid_response", http.StatusBadGateway)
		}
		key, err = s.createdKey(ctx, tenantID, owner, created.Data, requested)
		if err != nil {
			return nil, err
		}
	}
	if key.ID == 0 || key.APIKey == "" || key.FullAccess || key.ScopeType != "tenant" || len(key.Capabilities) != 1 || key.Capabilities[0] != "manage_members" {
		return nil, failure("workspace.member_credential_invalid", http.StatusBadGateway)
	}
	delegation.MemberKeyCipher, err = s.seal(tenantID, "member-key", []byte(key.APIKey))
	if err != nil {
		return nil, err
	}
	delegation.MemberKeyID = key.ID
	if err := store.SaveDelegation(ctx, delegation); err != nil {
		return nil, err
	}
	return memberKeyHeader(key.APIKey, tenantID), nil
}

func memberKeyHeader(secret string, tenantID uint64) http.Header {
	h := make(http.Header)
	h.Set("X-API-Key", secret)
	h.Set("X-Tenant-ID", strconv.FormatUint(tenantID, 10))
	return h
}

// memberOperation constructs a fresh allowlisted body, rather than forwarding
// caller JSON into an Owner-privileged request. DELETE never forwards a body.
func memberOperation(method, suffix string, raw json.RawMessage) (string, map[string]any, string, error) {
	input := map[string]any{}
	decode := func(value any) error {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(value); err != nil {
			return failure("workspace.request_invalid", http.StatusBadRequest)
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return failure("workspace.request_invalid", http.StatusBadRequest)
		}
		return nil
	}
	validRole := func(v string) bool { return v == "owner" || v == "admin" || v == "contributor" || v == "viewer" }
	if suffix == "" && method == http.MethodPut {
		var value struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		}
		if err := decode(&value); err != nil {
			return "", nil, "", err
		}
		if value.Name == nil && value.Description == nil {
			return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
		}
		if value.Name != nil {
			name := strings.TrimSpace(*value.Name)
			if name == "" || len(name) > 1024 {
				return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
			}
			input["name"] = name
		}
		if value.Description != nil {
			if len(*value.Description) > 16384 {
				return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
			}
			input["description"] = strings.TrimSpace(*value.Description)
		}
		return "workspace.update", input, "workspace", nil
	}
	if method == http.MethodPost && (suffix == "/members" || suffix == "/invitations" || suffix == "/invite-links") {
		var value struct {
			Email   string `json:"email"`
			Role    string `json:"role"`
			Message string `json:"message"`
		}
		if err := decode(&value); err != nil {
			return "", nil, "", err
		}
		if !validRole(value.Role) || len(value.Message) > 4096 {
			return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
		}
		input["role"] = value.Role
		if suffix != "/invite-links" {
			email := strings.TrimSpace(value.Email)
			address, err := mail.ParseAddress(email)
			if err != nil || address.Address != email || len(email) > 254 {
				return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
			}
			input["email"] = email
		} else if value.Email != "" {
			return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
		}
		if suffix != "/members" {
			input["message"] = value.Message
		} else if value.Message != "" {
			return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
		}
		return "workspace." + strings.TrimPrefix(suffix, "/") + ".create", input, strings.TrimPrefix(suffix, "/"), nil
	}
	parts := strings.Split(strings.TrimPrefix(suffix, "/"), "/")
	if len(parts) != 2 || !safeMemberSegment(parts[1]) {
		return "", nil, "", failure("workspace.route_invalid", http.StatusNotFound)
	}
	if parts[0] == "members" && method == http.MethodPut {
		var value struct {
			Role string `json:"role"`
		}
		if err := decode(&value); err != nil {
			return "", nil, "", err
		}
		if !validRole(value.Role) {
			return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
		}
		input["role"] = value.Role
		return "workspace.member.role", input, parts[1], nil
	}
	if method == http.MethodDelete && (parts[0] == "members" || parts[0] == "invitations") {
		if len(bytes.TrimSpace(raw)) != 0 && string(bytes.TrimSpace(raw)) != "{}" {
			return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
		}
		if parts[0] == "invitations" {
			id, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil || id == 0 {
				return "", nil, "", failure("workspace.request_invalid", http.StatusBadRequest)
			}
		}
		return "workspace." + parts[0] + ".delete", nil, parts[1], nil
	}
	return "", nil, "", failure("workspace.route_invalid", http.StatusNotFound)
}

func safeMemberSegment(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func (s *Service) memberTarget(ctx context.Context, tenantID uint64, h http.Header, id, email string) (*weknora.TenantMember, error) {
	seen := 0
	for page := 1; page <= 500; page++ {
		path := fmt.Sprintf("%s/members?page=%d&page_size=100", tenantPath(tenantID), page)
		if email != "" {
			path += "&q=" + url.QueryEscape(email)
		}
		raw, err := s.Upstream.EnterpriseRequest(ctx, http.MethodGet, path, h, nil)
		if err != nil {
			return nil, err
		}
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Members []weknora.TenantMember `json:"members"`
				Total   int                    `json:"total"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.Total < 0 {
			return nil, failure("upstream.invalid_response", http.StatusBadGateway)
		}
		for _, member := range response.Data.Members {
			if member.UserID == "" || (member.Role != "owner" && member.Role != "admin" && member.Role != "contributor" && member.Role != "viewer") {
				return nil, failure("upstream.invalid_response", http.StatusBadGateway)
			}
			if (id != "" && member.UserID == id) || (email != "" && strings.EqualFold(member.Email, email)) {
				return &member, nil
			}
		}
		seen += len(response.Data.Members)
		if seen >= response.Data.Total {
			return nil, nil
		}
		if len(response.Data.Members) == 0 {
			break
		}
	}
	return nil, failure("workspace.member_lookup_incomplete", http.StatusServiceUnavailable)
}

func (s *Service) checkInvitationTarget(ctx context.Context, tenantID uint64, h http.Header, target string) error {
	id, _ := strconv.ParseUint(target, 10, 64)
	seen := 0
	for page := 1; page <= 500; page++ {
		path := fmt.Sprintf("%s/invitations?page=%d&page_size=100", tenantPath(tenantID), page)
		raw, err := s.Upstream.EnterpriseRequest(ctx, http.MethodGet, path, h, nil)
		if err != nil {
			return err
		}
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Invitations []struct {
					ID            uint64 `json:"id"`
					TenantID      uint64 `json:"tenant_id"`
					Role          string `json:"role"`
					InviteeUserID string `json:"invitee_user_id"`
				} `json:"invitations"`
				Total int `json:"total"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.Total < 0 {
			return failure("upstream.invalid_response", http.StatusBadGateway)
		}
		for _, invitation := range response.Data.Invitations {
			if invitation.ID != id {
				continue
			}
			if invitation.TenantID != tenantID {
				return failure("workspace.invitation_not_found", http.StatusNotFound)
			}
			if invitation.Role == "owner" {
				return failure("workspace.owner_protected", http.StatusForbidden)
			}
			if invitation.Role != "admin" && invitation.Role != "contributor" && invitation.Role != "viewer" {
				return failure("upstream.invalid_response", http.StatusBadGateway)
			}
			if invitation.InviteeUserID != "" {
				member, err := s.memberTarget(ctx, tenantID, h, invitation.InviteeUserID, "")
				if err != nil {
					return err
				}
				if member != nil && member.Role == "owner" {
					return failure("workspace.owner_protected", http.StatusForbidden)
				}
			}
			return nil
		}
		seen += len(response.Data.Invitations)
		if seen >= response.Data.Total {
			return failure("workspace.invitation_not_found", http.StatusNotFound)
		}
		if len(response.Data.Invitations) == 0 {
			break
		}
	}
	return failure("workspace.invitation_lookup_incomplete", http.StatusServiceUnavailable)
}
