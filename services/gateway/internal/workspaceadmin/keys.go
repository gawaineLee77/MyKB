package workspaceadmin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const pendingPrefix = "mindcreek-pending:"
const internalPrefix = "mindcreek-internal:"

var adminCapabilities = map[string]bool{"retrieve": true, "chat": true, "read_agents": true, "ingest": true, "manage_kbs": true, "manage_agents": true, "manage_channels": true}

type keyInput struct {
	Name                string   `json:"name"`
	FullAccess          bool     `json:"full_access"`
	KnowledgeBaseIDs    []string `json:"knowledge_base_ids"`
	Capabilities        []string `json:"capabilities"`
	ExpiresAt           *int64   `json:"expires_at_unix"`
	KnowledgeOperations []string `json:"knowledge_operations,omitempty"`
	AllKnowledgeBases   bool     `json:"all_knowledge_bases,omitempty"`
}
type keyDTO struct {
	ID                  uint64     `json:"id"`
	ScopeType           string     `json:"scope_type"`
	Name                string     `json:"name"`
	APIKey              string     `json:"api_key"`
	Token               string     `json:"token,omitempty"`
	FullAccess          bool       `json:"full_access"`
	KnowledgeBaseIDs    []string   `json:"knowledge_base_ids"`
	Capabilities        []string   `json:"capabilities"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	CreatorUserID       string     `json:"creator_user_id,omitempty"`
	Ownership           string     `json:"ownership"`
	Restricted          bool       `json:"restricted"`
	Disabled            bool       `json:"disabled"`
	Protected           bool       `json:"protected,omitempty"`
	KnowledgeOperations []string   `json:"knowledge_operations"`
	AllKnowledgeBases   bool       `json:"all_knowledge_bases"`
	KnowledgeMapped     bool       `json:"knowledge_mapped"`
}

func strictInput(raw json.RawMessage) (keyInput, error) {
	var input keyInput
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil {
		return input, failure("workspace_admin.key_input_invalid", 400)
	}
	if d.Decode(new(any)) != io.EOF {
		return input, failure("workspace_admin.key_input_invalid", 400)
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 128 || strings.HasPrefix(input.Name, pendingPrefix) || strings.HasPrefix(input.Name, internalPrefix) {
		return input, failure("workspace_admin.key_name_invalid", 400)
	}
	if input.ExpiresAt != nil && *input.ExpiresAt <= time.Now().Unix() {
		return input, failure("workspace_admin.key_expiry_invalid", 400)
	}
	return input, nil
}
func (s *Service) validateInput(ctx context.Context, p weknora.Principal, h http.Header, input keyInput, restricted bool) error {
	if s.KnowledgeStore != nil {
		if err := s.validateKnowledgeInput(ctx, p, h, input, restricted); err != nil {
			return err
		}
	}
	if !restricted {
		return nil
	}
	if input.FullAccess || len(input.Capabilities) == 0 || len(input.KnowledgeBaseIDs) == 0 {
		return failure("workspace_admin.key_scope_required", 400)
	}
	for _, cap := range input.Capabilities {
		if !adminCapabilities[cap] {
			return failure("workspace_admin.key_capability_denied", 403)
		}
	}
	seen := map[string]bool{}
	for _, id := range input.KnowledgeBaseIDs {
		if id == "" || strings.TrimSpace(id) != id || seen[id] {
			return failure("workspace_admin.key_scope_invalid", 400)
		}
		seen[id] = true
	}
	if s.ValidateKeyScope == nil {
		return failure("workspace_admin.key_scope_unavailable", 503)
	}
	return s.ValidateKeyScope(ctx, p, h, input.KnowledgeBaseIDs)
}
func (s *Service) nativeKeys(ctx context.Context, id uint64, h http.Header) ([]keyDTO, error) {
	raw, err := s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(id)+"/api-keys", h, nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Success bool     `json:"success"`
		Data    []keyDTO `json:"data"`
	}
	if json.Unmarshal(raw, &r) != nil || !r.Success || r.Data == nil {
		return nil, failure("workspace_admin.key_response_invalid", 502)
	}
	for _, k := range r.Data {
		if k.ID == 0 || k.ScopeType != "tenant" {
			return nil, failure("workspace_admin.key_response_invalid", 502)
		}
	}
	return r.Data, nil
}

// The pinned native create response can contain the database ciphertext in
// api_key: GORM's BeforeSave hook mutates that field, while token remains the
// freshly generated plaintext. A tenant-scoped list runs AfterFind and returns
// the decrypted credential. Bind both responses before persisting or returning
// it; never decrypt upstream secrets or assume two different values are valid.
func (s *Service) createdKey(ctx context.Context, tenant uint64, h http.Header, created keyDTO, requested keyInput) (keyDTO, error) {
	secret := created.Token
	if secret == "" {
		secret = created.APIKey
	}
	kbs, capabilities := requested.KnowledgeBaseIDs, requested.Capabilities
	if requested.FullAccess {
		kbs, capabilities = nil, nil
	}
	if created.ID == 0 || created.ScopeType != "tenant" || created.Name != requested.Name || secret == "" ||
		created.FullAccess != requested.FullAccess || !slices.Equal(created.KnowledgeBaseIDs, kbs) ||
		!slices.Equal(created.Capabilities, capabilities) || (created.ExpiresAt == nil) != (requested.ExpiresAt == nil) ||
		(created.ExpiresAt != nil && created.ExpiresAt.Unix() != *requested.ExpiresAt) {
		return keyDTO{}, failure("workspace_admin.key_response_invalid", 502)
	}
	keys, err := s.nativeKeys(ctx, tenant, h)
	if err != nil {
		return keyDTO{}, err
	}
	var verified keyDTO
	for _, candidate := range keys {
		if candidate.ID != created.ID {
			continue
		}
		if verified.ID != 0 || candidate.ScopeType != created.ScopeType || candidate.Name != created.Name ||
			candidate.APIKey != secret || candidate.FullAccess != created.FullAccess ||
			!slices.Equal(candidate.KnowledgeBaseIDs, created.KnowledgeBaseIDs) ||
			!slices.Equal(candidate.Capabilities, created.Capabilities) ||
			(candidate.ExpiresAt == nil) != (created.ExpiresAt == nil) ||
			(candidate.ExpiresAt != nil && !candidate.ExpiresAt.Equal(*created.ExpiresAt)) {
			return keyDTO{}, failure("workspace_admin.key_response_invalid", 502)
		}
		verified = candidate
	}
	if verified.ID == 0 {
		return keyDTO{}, failure("workspace_admin.key_response_invalid", 502)
	}
	verified.Token = secret
	return verified, nil
}
func (s *Service) protected(ctx context.Context, store Store, id uint64, k keyDTO) (bool, error) {
	if strings.HasPrefix(k.Name, internalPrefix) {
		return true, nil
	}
	d, err := store.Delegation(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return true, err
	}
	if k.ID == d.MemberKeyID {
		return true, nil
	}
	if s.ProtectedKey != nil {
		return s.ProtectedKey(ctx, id, k.ID)
	}
	return false, nil
}
func decorate(k keyDTO, record KeyOwner, p weknora.Principal) keyDTO {
	k.Ownership = "legacy"
	if record.CreatorUserID != "" {
		k.CreatorUserID = record.CreatorUserID
		k.Ownership = "other"
		if p.User != nil && p.User.ID == record.CreatorUserID {
			k.Ownership = "self"
		}
		k.Restricted = record.RequiredRole == "admin"
		k.Disabled = record.Disabled
	}
	if k.Disabled || k.Protected {
		k.APIKey = ""
		k.Token = ""
	}
	return k
}

// recoverPending runs under the shared workspace lock. The durable marker and
// returned ID cover both unknown POST outcomes and crashes after the rename.
// No orphan secret is ever surfaced as an ordinary legacy credential.
func (s *Service) recoverPending(ctx context.Context, store Store, id uint64, h http.Header) error {
	pending, err := store.PendingCreates(ctx, id)
	if err != nil || len(pending) == 0 {
		return err
	}
	keys, err := s.nativeKeys(ctx, id, h)
	if err != nil {
		return err
	}
	for _, attempt := range pending {
		found := false
		for _, key := range keys {
			if key.Name != attempt.Marker && key.ID != attempt.KeyID {
				continue
			}
			found = true
			if err := store.DisableKey(ctx, id, key.ID); err != nil {
				return err
			}
			if _, err := s.Upstream.EnterpriseRequest(ctx, "DELETE", tenantPath(id)+"/api-keys/"+strconv.FormatUint(key.ID, 10), h, nil); err != nil {
				return failure("workspace_admin.key_recovery_required", 503)
			}
		}
		// A timed-out POST may still commit after this list snapshot. Keep
		// its marker until observed; never turn a delayed write into an
		// untracked key by interpreting absence as proof of no creation.
		if !found && attempt.KeyID == 0 {
			continue
		}
		if err := store.DeletePending(ctx, id, attempt.Marker); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) ListKeys(ctx context.Context, p weknora.Principal, h http.Header, id uint64) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.withSpace(ctx, id, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, id)
		if err != nil {
			return err
		}
		headers, err := s.ownerHeaders(ctx, store, fresh, h, id)
		if err != nil {
			return err
		}
		if err := s.recoverPending(ctx, store, id, headers); err != nil {
			return err
		}
		keys, err := s.nativeKeys(ctx, id, headers)
		if err != nil {
			return err
		}
		out := make([]keyDTO, 0, len(keys))
		for _, key := range keys {
			if strings.HasPrefix(key.Name, pendingPrefix) {
				continue
			}
			record, err := store.Key(ctx, id, key.ID)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			protected, err := s.protected(ctx, store, id, key)
			if err != nil {
				return err
			}
			key.Protected = protected
			if role(fresh, id) != "owner" && (protected || record.CreatorUserID != fresh.User.ID) {
				continue
			}
			key = decorate(key, record, fresh)
			if err := s.decorateKnowledgeKey(ctx, id, &key); err != nil {
				return err
			}
			out = append(out, key)
		}
		result, err = json.Marshal(map[string]any{"success": true, "data": out})
		return err
	})
	return result, err
}
func (s *Service) CreateKey(ctx context.Context, p weknora.Principal, h http.Header, id uint64, raw json.RawMessage) (json.RawMessage, error) {
	input, err := strictInput(raw)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	err = s.withSpace(ctx, id, func(store Store) error {
		if p.User == nil {
			return failure("workspace_admin.human_required", 403)
		}
		generation, err := store.CreatorGeneration(ctx, p.User.ID)
		if err != nil {
			return err
		}
		fresh, err := s.freshPrincipal(ctx, p, h, id)
		if err != nil {
			return err
		}
		restricted := role(fresh, id) != "owner"
		if err := s.validateInput(ctx, fresh, h, input, restricted); err != nil {
			return err
		}
		// Every product-created key needs a live verifier for creator revocation,
		// including Owner-created keys. Do not issue credentials that cannot work.
		if _, err := s.delegatedHeaders(ctx, store, id); err != nil {
			return err
		}
		headers, err := s.ownerHeaders(ctx, store, fresh, h, id)
		if err != nil {
			return err
		}
		if err := s.recoverPending(ctx, store, id, headers); err != nil {
			return err
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "key.create", "", "attempt"); err != nil {
			return err
		}
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		marker := pendingPrefix + hex.EncodeToString(nonce)
		if err := store.SavePending(ctx, id, marker, 0); err != nil {
			return err
		}
		temporary := input
		temporary.Name = marker
		body, callErr := s.Upstream.EnterpriseRequest(ctx, "POST", tenantPath(id)+"/api-keys", headers, temporary.native())
		var response struct {
			Success bool   `json:"success"`
			Data    keyDTO `json:"data"`
		}
		if callErr != nil || json.Unmarshal(body, &response) != nil || !response.Success || response.Data.ID == 0 || response.Data.ScopeType != "tenant" {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return failure("workspace_admin.key_creation_unknown", 503)
		}
		key, err := s.createdKey(ctx, id, headers, response.Data, temporary)
		if err != nil {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return err
		}
		if err := store.SavePending(ctx, id, marker, key.ID); err != nil {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return err
		}
		requiredRole := "owner"
		if restricted {
			requiredRole = "admin"
		}
		record := KeyOwner{TenantID: id, KeyID: key.ID, CreatorUserID: fresh.User.ID, RequiredRole: requiredRole, Fingerprint: fingerprint(key.Token), CreatorGeneration: generation}
		if err := store.SaveKey(ctx, record); err != nil {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return failure("workspace_admin.key_attribution_failed", 503)
		}
		if s.KnowledgeStore != nil {
			if err := s.KnowledgeStore.SaveMachinePolicy(ctx, input.machinePolicy(record)); err != nil {
				_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
				return err
			}
		}
		if _, err := s.Upstream.EnterpriseRequest(ctx, "PUT", tenantPath(id)+"/api-keys/"+strconv.FormatUint(key.ID, 10), headers, input.native()); err != nil {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return failure("workspace_admin.key_creation_unknown", 503)
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "key.create", strconv.FormatUint(key.ID, 10), "success"); err != nil {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return err
		}
		if err := store.DeletePending(ctx, id, marker); err != nil {
			_ = s.recoverPending(context.WithoutCancel(ctx), store, id, headers)
			return err
		}
		key.Name = input.Name
		key = decorate(key, record, fresh)
		if err := s.decorateKnowledgeKey(ctx, id, &key); err != nil {
			return err
		}
		result, err = json.Marshal(map[string]any{"success": true, "data": key})
		return err
	})
	return result, err
}
func (s *Service) targetKey(ctx context.Context, store Store, p weknora.Principal, id, keyID uint64, h http.Header) (keyDTO, KeyOwner, error) {
	record, err := store.Key(ctx, id, keyID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return keyDTO{}, record, err
	}
	if role(p, id) != "owner" && record.CreatorUserID != p.User.ID {
		return keyDTO{}, record, failure("workspace_admin.key_not_found", 404)
	}
	keys, err := s.nativeKeys(ctx, id, h)
	if err != nil {
		return keyDTO{}, record, err
	}
	for _, key := range keys {
		if key.ID != keyID {
			continue
		}
		protected, err := s.protected(ctx, store, id, key)
		if err != nil {
			return key, record, err
		}
		if protected || strings.HasPrefix(key.Name, pendingPrefix) {
			return key, record, failure("workspace_admin.key_protected", 403)
		}
		return key, record, nil
	}
	return keyDTO{}, record, failure("workspace_admin.key_not_found", 404)
}
func (s *Service) UpdateKey(ctx context.Context, p weknora.Principal, h http.Header, id, keyID uint64, raw json.RawMessage) (json.RawMessage, error) {
	input, err := strictInput(raw)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	err = s.withSpace(ctx, id, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, id)
		if err != nil {
			return err
		}
		headers, err := s.ownerHeaders(ctx, store, fresh, h, id)
		if err != nil {
			return err
		}
		if err := s.recoverPending(ctx, store, id, headers); err != nil {
			return err
		}
		existing, record, err := s.targetKey(ctx, store, fresh, id, keyID, headers)
		if err != nil {
			return err
		}
		if record.Disabled {
			return failure("workspace_admin.key_disabled", 403)
		}
		// A different Owner cannot validate private resources as the original
		// Admin. Such edits may narrow the established scope, never expand it.
		if record.RequiredRole == "admin" && record.CreatorUserID != fresh.User.ID {
			allowed := map[string]bool{}
			for _, kb := range existing.KnowledgeBaseIDs {
				allowed[kb] = true
			}
			for _, kb := range input.KnowledgeBaseIDs {
				if !allowed[kb] {
					return failure("workspace_admin.creator_scope_expansion_denied", 403)
				}
			}
		}
		if err := s.validateInput(ctx, fresh, h, input, role(fresh, id) != "owner" || record.RequiredRole == "admin"); err != nil {
			return err
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "key.update", strconv.FormatUint(keyID, 10), "attempt"); err != nil {
			return err
		}
		if s.KnowledgeStore != nil {
			if record.CreatorUserID == "" {
				if _, err := s.delegatedHeaders(ctx, store, id); err != nil {
					return err
				}
				secret := existing.APIKey
				if secret == "" {
					secret = existing.Token
				}
				if secret == "" {
					return failure("workspace_admin.key_recreate_required", 409)
				}
				generation, err := store.CreatorGeneration(ctx, fresh.User.ID)
				if err != nil {
					return err
				}
				record = KeyOwner{TenantID: id, KeyID: keyID, CreatorUserID: fresh.User.ID, RequiredRole: "owner", Fingerprint: fingerprint(secret), CreatorGeneration: generation}
				if err := store.SaveKey(ctx, record); err != nil {
					return err
				}
			}
			// An unknown native update outcome must not leave a stale product grant.
			if err := s.KnowledgeStore.DeleteMachinePolicy(ctx, id, keyID); err != nil {
				return err
			}
		}
		body, err := s.Upstream.EnterpriseRequest(ctx, "PUT", tenantPath(id)+"/api-keys/"+strconv.FormatUint(keyID, 10), headers, input.native())
		if err != nil {
			return err
		}
		var response struct {
			Success bool   `json:"success"`
			Data    keyDTO `json:"data"`
		}
		if json.Unmarshal(body, &response) != nil || !response.Success || response.Data.ID != keyID {
			return failure("workspace_admin.key_response_invalid", 502)
		}
		if s.KnowledgeStore != nil {
			if err := s.KnowledgeStore.SaveMachinePolicy(ctx, input.machinePolicy(record)); err != nil {
				return err
			}
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "key.update", strconv.FormatUint(keyID, 10), "success"); err != nil {
			return err
		}
		key := decorate(response.Data, record, fresh)
		if err := s.decorateKnowledgeKey(ctx, id, &key); err != nil {
			return err
		}
		result, err = json.Marshal(map[string]any{"success": true, "data": key})
		return err
	})
	return result, err
}
func (s *Service) RevokeKey(ctx context.Context, p weknora.Principal, h http.Header, id, keyID uint64) error {
	return s.withSpace(ctx, id, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, id)
		if err != nil {
			return err
		}
		headers, err := s.ownerHeaders(ctx, store, fresh, h, id)
		if err != nil {
			return err
		}
		if err := s.recoverPending(ctx, store, id, headers); err != nil {
			return err
		}
		if _, _, err := s.targetKey(ctx, store, fresh, id, keyID, headers); err != nil {
			return err
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "key.revoke", strconv.FormatUint(keyID, 10), "attempt"); err != nil {
			return err
		}
		if err := store.DisableKey(ctx, id, keyID); err != nil {
			return err
		}
		if _, err := s.Upstream.EnterpriseRequest(ctx, "DELETE", tenantPath(id)+"/api-keys/"+strconv.FormatUint(keyID, 10), headers, nil); err != nil {
			return err
		}
		return s.audit(ctx, store, id, fresh.User.ID, "key.revoke", strconv.FormatUint(keyID, 10), "success")
	})
}

// CheckKeyUse is an additional gate, never the credential authenticator. Native
// authentication still verifies the secret, expiry, capabilities and KB scope.
// tenantID=0 lets the binding supply the tenant before native authentication.
func (s *Service) CheckKeyUse(ctx context.Context, rawKey string, tenantID uint64) error {
	record, err := s.Store.KeyByFingerprint(ctx, fingerprint(rawKey))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return failure("workspace_admin.key_verification_unavailable", 503)
	}
	if tenantID != 0 && tenantID != record.TenantID {
		return failure("workspace_admin.key_tenant_denied", 403)
	}
	return s.withSpace(ctx, record.TenantID, func(store Store) error { return s.checkKeyUseLocked(ctx, store, rawKey, record.TenantID) })
}

// checkKeyUseLocked rechecks a machine caller inside a workspace mutation's
// existing advisory lock. Reacquiring that lock through another connection
// would deadlock; checking only before the lock would permit stale authority.
func (s *Service) checkKeyUseLocked(ctx context.Context, store Store, rawKey string, tenantID uint64) error {
	return s.checkKeyUse(ctx, store, rawKey, tenantID, true)
}

func (s *Service) checkKeyUse(ctx context.Context, store Store, rawKey string, tenantID uint64, allowRefresh bool) error {
	record, err := store.KeyByFingerprint(ctx, fingerprint(rawKey))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return failure("workspace_admin.key_verification_unavailable", 503)
	}
	if tenantID != record.TenantID {
		return failure("workspace_admin.key_tenant_denied", 403)
	}
	if record.Disabled {
		return failure("workspace_admin.key_disabled", 403)
	}
	generation, err := store.CreatorGeneration(ctx, record.CreatorUserID)
	if err != nil {
		return failure("workspace_admin.key_verification_unavailable", 503)
	}
	if generation != record.CreatorGeneration {
		if err := store.DisableKey(ctx, record.TenantID, record.KeyID); err != nil {
			return err
		}
		return failure("workspace_admin.key_disabled", 403)
	}
	if s.IdentityAllowed == nil {
		return failure("workspace_admin.identity_unavailable", 503)
	}
	if err := s.IdentityAllowed(ctx, record.CreatorUserID); err != nil {
		var denied *Error
		if errors.As(err, &denied) && denied.Status == 403 {
			if err := store.DisableKey(ctx, record.TenantID, record.KeyID); err != nil {
				return failure("workspace_admin.key_verification_unavailable", 503)
			}
			return failure("workspace_admin.key_creator_denied", 403)
		}
		return failure("workspace_admin.identity_unavailable", 503)
	}
	headers, err := s.validatedDelegatedHeaders(ctx, store, record.TenantID, "session", allowRefresh)
	if err != nil {
		return err
	}
	active, err := s.creatorMembership(ctx, record, headers)
	if err != nil {
		return err
	}
	if !active {
		if err := store.DisableKey(ctx, record.TenantID, record.KeyID); err != nil {
			return failure("workspace_admin.key_verification_unavailable", 503)
		}
		return failure("workspace_admin.key_creator_denied", 403)
	}
	return nil
}
func (s *Service) creatorMembership(ctx context.Context, record KeyOwner, h http.Header) (bool, error) {
	for page := 1; page <= 1000; page++ {
		raw, err := s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(record.TenantID)+"/members?page="+strconv.Itoa(page)+"&page_size=100", h, nil)
		if err != nil {
			return false, failure("workspace_admin.key_verification_unavailable", 503)
		}
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Members []weknora.TenantMember `json:"members"`
				Total   int                    `json:"total"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.Members == nil || response.Data.Total < 0 {
			return false, failure("workspace_admin.key_verification_unavailable", 503)
		}
		for _, m := range response.Data.Members {
			if m.UserID != record.CreatorUserID {
				continue
			}
			return m.Status == "active" && (m.Role == "owner" || record.RequiredRole == "admin" && m.Role == "admin"), nil
		}
		if page*100 >= response.Data.Total {
			return false, nil
		}
		if len(response.Data.Members) == 0 {
			return false, failure("workspace_admin.key_verification_unavailable", 503)
		}
	}
	return false, failure("workspace_admin.key_verification_unavailable", 503)
}
