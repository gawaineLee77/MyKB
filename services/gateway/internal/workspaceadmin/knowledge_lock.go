package workspaceadmin

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
)

type orderedLockStore interface {
	WithLocks(context.Context, []string, func(Store) error) error
}

// WithKnowledgeLock coordinates accepted writes and ACL updates with the same
// locks used by member changes and identity suspension, across replicas.
func (s *Service) WithKnowledgeLock(ctx context.Context, h http.Header, kbs []string, fn func() error) error {
	locks, ok := s.Store.(orderedLockStore)
	if !ok {
		return failure("kb_acl.lock_unavailable", 503)
	}
	p, err := s.Upstream.CurrentPrincipal(ctx, h)
	if err != nil {
		return err
	}
	if p.Tenant == nil || p.Tenant.ID == 0 {
		return failure("kb_acl.workspace_required", 403)
	}
	tenant := p.Tenant.ID
	if requested := h.Get("X-Tenant-ID"); requested != "" && requested != strconv.FormatUint(tenant, 10) {
		return failure("kb_acl.not_found", 404)
	}
	users := map[string]bool{}
	if h.Get("X-API-Key") != "" {
		if h.Get("Authorization") != "" {
			return failure("auth.ambiguous_credentials", 400)
		}
		record, err := s.Store.KeyByFingerprint(ctx, fingerprint(h.Get("X-API-Key")))
		if err != nil {
			return failure("kb_acl.key_mapping_required", 403)
		}
		if record.TenantID != tenant {
			return failure("kb_acl.not_found", 404)
		}
		users[record.CreatorUserID] = true
	} else {
		if p.User == nil || !p.User.IsActive || role(p, tenant) == "" {
			return failure("kb_acl.membership_required", 403)
		}
		users[p.User.ID] = true
	}
	delegation, err := s.Store.Delegation(ctx, tenant)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if delegation.OwnerUserID != "" {
		users[delegation.OwnerUserID] = true
	}
	var execution Delegation
	if s.KnowledgeStore != nil {
		execution, err = s.KnowledgeStore.ExecutionDelegation(ctx, tenant)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if execution.OwnerUserID != "" {
			users[execution.OwnerUserID] = true
		}
	}
	keys := []string{}
	for user := range users {
		if user != "" {
			keys = append(keys, "identity-user:"+user)
		}
	}
	sort.Strings(keys)
	keys = append(keys, "workspace-admin:"+strconv.FormatUint(tenant, 10))
	sorted := append([]string{}, kbs...)
	sort.Strings(sorted)
	for _, kb := range sorted {
		if !safeMemberSegment(kb) {
			return failure("kb_acl.invalid_id", 400)
		}
		keys = append(keys, "kb-acl:"+strconv.FormatUint(tenant, 10)+":"+kb)
	}
	return locks.WithLocks(ctx, keys, func(Store) error {
		// Delegations may have changed while waiting for the space lock. Do
		// not use a new owner whose identity lock this request did not acquire.
		d, err := s.Store.Delegation(ctx, tenant)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if d.OwnerUserID != "" && !users[d.OwnerUserID] {
			return failure("kb_acl.authorization_retry", 409)
		}
		if s.KnowledgeStore != nil {
			d, err = s.KnowledgeStore.ExecutionDelegation(ctx, tenant)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			if d.OwnerUserID != "" && !users[d.OwnerUserID] {
				return failure("kb_acl.authorization_retry", 409)
			}
		}
		return fn()
	})
}
