package workspaceadmin

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

var knowledgeOperations = map[string]bool{
	"document.create": true, "document.update": true, "document.delete": true, "document.download": true,
	"chunk.update": true, "chunk.delete": true, "chunk.export": true,
}

// Product fields must never be sent into the upstream key DTO.
func (i keyInput) native() keyInput {
	i.KnowledgeOperations = nil
	i.AllKnowledgeBases = false
	return i
}
func (i keyInput) machinePolicy(k KeyOwner) MachinePolicy {
	return MachinePolicy{TenantID: k.TenantID, KeyID: k.KeyID, Fingerprint: k.Fingerprint, KnowledgeBaseIDs: i.KnowledgeBaseIDs, Operations: i.KnowledgeOperations, AllKnowledgeBases: i.AllKnowledgeBases}
}
func (s *Service) validateKnowledgeInput(ctx context.Context, p weknora.Principal, h http.Header, i keyInput, restricted bool) error {
	seen := map[string]bool{}
	for _, op := range i.KnowledgeOperations {
		if !knowledgeOperations[op] || seen[op] {
			return failure("kb_acl.key_operations_invalid", 400)
		}
		seen[op] = true
	}
	if i.AllKnowledgeBases && (restricted || role(p, p.Tenant.ID) != "owner") {
		return failure("kb_acl.key_all_scope_owner_required", 403)
	}
	if i.AllKnowledgeBases && len(i.KnowledgeBaseIDs) > 0 {
		return failure("kb_acl.key_scope_ambiguous", 400)
	}
	if len(i.KnowledgeOperations) > 0 && !i.AllKnowledgeBases && len(i.KnowledgeBaseIDs) == 0 {
		return failure("kb_acl.key_scope_required", 400)
	}
	// Even Owner-selected explicit IDs must belong to the current workspace.
	if !restricted && len(i.KnowledgeBaseIDs) > 0 {
		if s.ValidateKeyScope == nil {
			return failure("kb_acl.key_scope_unavailable", 503)
		}
		if err := s.ValidateKeyScope(ctx, p, h, i.KnowledgeBaseIDs); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) decorateKnowledgeKey(ctx context.Context, tenant uint64, k *keyDTO) error {
	k.KnowledgeOperations = []string{}
	if s.KnowledgeStore == nil || k.Protected {
		return nil
	}
	p, err := s.KnowledgeStore.MachinePolicy(ctx, tenant, k.ID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	k.KnowledgeMapped = true
	k.AllKnowledgeBases = p.AllKnowledgeBases
	k.KnowledgeOperations = p.Operations
	return nil
}

// MachineKnowledgePolicy is called after native authentication. It does not
// acquire a second workspace lock: the write guard already holds it, while
// read/retrieval callers recheck current state on each use without refreshing
// or persisting delegation credentials. Native capabilities
// and native KB scopes remain an additional intersection, never replaced here.
func (s *Service) MachineKnowledgePolicy(ctx context.Context, h http.Header, tenant uint64, kb string) (MachinePolicy, error) {
	if s.KnowledgeStore == nil || h.Get("X-API-Key") == "" || h.Get("Authorization") != "" {
		return MachinePolicy{}, failure("kb_acl.machine_scope_required", 403)
	}
	record, err := s.Store.KeyByFingerprint(ctx, fingerprint(h.Get("X-API-Key")))
	if errors.Is(err, ErrNotFound) {
		return MachinePolicy{}, failure("kb_acl.key_mapping_required", 403)
	}
	if err != nil {
		return MachinePolicy{}, err
	}
	if record.TenantID != tenant {
		return MachinePolicy{}, failure("kb_acl.not_found", 404)
	}
	if err := s.checkKeyUse(ctx, s.Store, h.Get("X-API-Key"), tenant, false); err != nil {
		return MachinePolicy{}, err
	}
	p, err := s.KnowledgeStore.MachinePolicy(ctx, tenant, record.KeyID)
	if errors.Is(err, ErrNotFound) {
		return p, failure("kb_acl.key_mapping_required", 403)
	}
	if err != nil {
		return p, err
	}
	if p.Fingerprint != record.Fingerprint || (!p.AllKnowledgeBases && !slices.Contains(p.KnowledgeBaseIDs, kb)) {
		return MachinePolicy{}, failure("kb_acl.not_found", 404)
	}
	return p, nil
}
