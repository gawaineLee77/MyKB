package workspaceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type executionStore struct {
	Store
	KnowledgeStore
}

func (s executionStore) Delegation(ctx context.Context, id uint64) (Delegation, error) {
	return s.ExecutionDelegation(ctx, id)
}
func (s executionStore) SaveDelegation(ctx context.Context, d Delegation) error {
	return s.SaveExecutionDelegation(ctx, d)
}

func (s *Service) SetKnowledgeDelegation(ctx context.Context, p weknora.Principal, h http.Header, tenant uint64) (DelegationStatus, error) {
	status := DelegationStatus{Configured: s.aead != nil && s.KnowledgeStore != nil}
	err := s.withSpace(ctx, tenant, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, tenant)
		if err != nil {
			return err
		}
		if role(fresh, tenant) != "owner" {
			return failure("workspace_admin.owner_required", 403)
		}
		if !status.Configured {
			return failure("kb_acl.execution_unavailable", 503)
		}
		if err := s.audit(ctx, store, tenant, fresh.User.ID, "kb_execution.authorize", "", "attempt"); err != nil {
			return err
		}
		raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/switch-tenant", h, map[string]uint64{"tenant_id": tenant})
		if err != nil {
			return err
		}
		session, err := parseSession(raw)
		if err != nil {
			return err
		}
		if "Bearer "+session.Access == h.Get("Authorization") {
			return failure("workspace_admin.delegation_retry", 409)
		}
		if err := s.checkOwner(ctx, fresh.User.ID, tenant, ownerHeader(session.Access, tenant)); err != nil {
			return err
		}
		plain, _ := json.Marshal(session)
		cipher, err := s.seal(tenant, "knowledge-session", plain)
		if err != nil {
			return err
		}
		d := Delegation{TenantID: tenant, OwnerUserID: fresh.User.ID, SessionCipher: cipher}
		if err := s.KnowledgeStore.SaveExecutionDelegation(ctx, d); err != nil {
			return err
		}
		if err := s.audit(ctx, store, tenant, fresh.User.ID, "kb_execution.authorize", "", "success"); err != nil {
			return err
		}
		now := time.Now().UTC()
		status.Authorized = true
		status.OwnerUserID = fresh.User.ID
		status.UpdatedAt = &now
		return nil
	})
	return status, err
}
func (s *Service) GetKnowledgeDelegationStatus(ctx context.Context, p weknora.Principal, h http.Header, tenant uint64) (DelegationStatus, error) {
	status := DelegationStatus{Configured: s.aead != nil && s.KnowledgeStore != nil}
	err := s.withSpace(ctx, tenant, func(store Store) error {
		if _, err := s.freshPrincipal(ctx, p, h, tenant); err != nil {
			return err
		}
		if !status.Configured {
			return nil
		}
		d, err := s.KnowledgeStore.ExecutionDelegation(ctx, tenant)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		status.OwnerUserID = d.OwnerUserID
		status.UpdatedAt = &d.UpdatedAt
		if _, err := s.executionOwnerHeaders(ctx, tenant); err != nil {
			status.ErrorCode = PublicError(err)
			return nil
		}
		status.Authorized = true
		return nil
	})
	return status, err
}
func (s *Service) RevokeKnowledgeDelegation(ctx context.Context, p weknora.Principal, h http.Header, tenant uint64) error {
	return s.withSpace(ctx, tenant, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, tenant)
		if err != nil {
			return err
		}
		if role(fresh, tenant) != "owner" {
			return failure("workspace_admin.owner_required", 403)
		}
		if s.KnowledgeStore == nil {
			return failure("kb_acl.execution_unavailable", 503)
		}
		if err := s.audit(ctx, store, tenant, fresh.User.ID, "kb_execution.revoke", "", "attempt"); err != nil {
			return err
		}
		// Disable before remote calls, retain key records until all revocations
		// succeed, so an outage cannot revive execution or lose retry targets.
		if err := s.KnowledgeStore.DeleteExecutionDelegation(ctx, tenant); err != nil {
			return err
		}
		keys, err := s.KnowledgeStore.ExecutionKeys(ctx, tenant)
		if err != nil {
			return err
		}
		for _, k := range keys {
			if _, err := s.Upstream.EnterpriseRequest(ctx, "DELETE", tenantPath(tenant)+"/api-keys/"+strconv.FormatUint(k.KeyID, 10), h, nil); err != nil {
				var e *weknora.Error
				if !errors.As(err, &e) || e.StatusCode != 404 {
					return failure("kb_acl.execution_revoke_failed", 503)
				}
			}
		}
		if err := s.KnowledgeStore.DeleteExecutionKeys(ctx, tenant); err != nil {
			return err
		}
		return s.audit(ctx, store, tenant, fresh.User.ID, "kb_execution.revoke", "", "success")
	})
}
func (s *Service) executionOwnerHeaders(ctx context.Context, tenant uint64) (http.Header, error) {
	if s.KnowledgeStore == nil {
		return nil, failure("kb_acl.execution_unavailable", 503)
	}
	return s.delegatedHeadersFor(ctx, executionStore{s.Store, s.KnowledgeStore}, tenant, "knowledge-session")
}

// WithKnowledgeExecutor is exclusively called by the content route adapter
// after operation/field/ancestry checks, under identity -> workspace -> KB
// locks. It never grants a caller a reusable credential or a generic proxy.
func (s *Service) WithKnowledgeExecutor(ctx context.Context, h http.Header, kb string, operations []string, fn func(http.Header) error) error {
	if len(operations) == 0 || !safeMemberSegment(kb) || h.Get("X-API-Key") != "" {
		return failure("kb_acl.execution_denied", 403)
	}
	for _, op := range operations {
		if !knowledgeOperations[op] {
			return failure("kb_acl.execution_denied", 403)
		}
	}
	p, err := s.Upstream.CurrentPrincipal(ctx, h)
	if err != nil {
		return err
	}
	if p.User == nil || !p.User.IsActive || p.Tenant == nil || role(p, p.Tenant.ID) != "contributor" {
		return failure("kb_acl.execution_denied", 403)
	}
	if s.IdentityAllowed == nil {
		return failure("kb_acl.identity_unavailable", 503)
	}
	if err := s.IdentityAllowed(ctx, p.User.ID); err != nil {
		return err
	}
	owner, err := s.executionOwnerHeaders(ctx, p.Tenant.ID)
	if err != nil {
		return err
	}
	forward, err := s.executionHeaders(ctx, p.Tenant.ID, kb, owner)
	if err != nil {
		return err
	}
	if err := s.audit(ctx, s.Store, p.Tenant.ID, p.User.ID, "kb_execution.write", kb, "attempt"); err != nil {
		return err
	}
	err = fn(forward)
	outcome := "accepted"
	if err != nil {
		outcome = "failure"
	}
	auditErr := s.audit(context.WithoutCancel(ctx), s.Store, p.Tenant.ID, p.User.ID, "kb_execution.write", kb, outcome)
	if err != nil {
		return err
	}
	return auditErr
}
func (s *Service) executionHeaders(ctx context.Context, tenant uint64, kb string, owner http.Header) (http.Header, error) {
	raw, err := s.Upstream.EnterpriseRequest(ctx, "GET", tenantPath(tenant)+"/api-principal-config", owner, nil)
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
	if json.Unmarshal(raw, &config) != nil || !config.Success || config.Data == nil {
		return nil, failure("upstream.invalid_response", 502)
	}
	// Signed/external principal modes cannot accept a synthetic human header.
	// A checked delegated Owner session remains constrained by the same fixed
	// route adapter and real-actor audit, never an externally accessible proxy.
	if config.Data.Mode == "signed_token" || config.Data.Mode == "direct_header" && config.Data.RequireDirectHeader {
		return owner, nil
	}
	if config.Data.Mode != "tenant" && config.Data.Mode != "direct_header" {
		return nil, failure("upstream.invalid_response", 502)
	}
	const prefix = "mindcreek-internal:kb-content:"
	keys, err := s.nativeKeys(ctx, tenant, owner)
	if err != nil {
		return nil, err
	}
	var key keyDTO
	for _, candidate := range keys {
		if candidate.Name == prefix+kb {
			if key.ID != 0 {
				return nil, failure("kb_acl.execution_key_conflict", 409)
			}
			key = candidate
		}
	}
	if key.ID == 0 {
		requested := keyInput{Name: prefix + kb, KnowledgeBaseIDs: []string{kb}, Capabilities: []string{"ingest", "retrieve"}}
		raw, err = s.Upstream.EnterpriseRequest(ctx, "POST", tenantPath(tenant)+"/api-keys", owner, requested)
		if err != nil {
			return nil, err
		}
		var created struct {
			Success bool   `json:"success"`
			Data    keyDTO `json:"data"`
		}
		if json.Unmarshal(raw, &created) != nil || !created.Success {
			return nil, failure("upstream.invalid_response", 502)
		}
		key, err = s.createdKey(ctx, tenant, owner, created.Data, requested)
		if err != nil {
			return nil, err
		}
	}
	if key.ID == 0 || key.APIKey == "" || key.ScopeType != "tenant" || key.FullAccess || len(key.KnowledgeBaseIDs) != 1 || key.KnowledgeBaseIDs[0] != kb || len(key.Capabilities) != 2 || !slices.Contains(key.Capabilities, "ingest") || !slices.Contains(key.Capabilities, "retrieve") {
		return nil, failure("kb_acl.execution_key_invalid", 502)
	}
	cipher, err := s.seal(tenant, "knowledge-key:"+kb, []byte(key.APIKey))
	if err != nil {
		return nil, err
	}
	if err := s.KnowledgeStore.SaveExecutionKey(ctx, ExecutionKey{TenantID: tenant, KBID: kb, KeyID: key.ID, Cipher: cipher}); err != nil {
		return nil, err
	}
	return memberKeyHeader(key.APIKey, tenant), nil
}
