package kbacl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type kbIdentity struct {
	ID          string `json:"id"`
	TenantID    uint64 `json:"tenant_id"`
	CreatorID   string `json:"creator_id"`
	IsTemporary bool   `json:"is_temporary"`
}

func validID(v string) bool {
	if v == "" || len(v) > 128 {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func nativeRole(p weknora.Principal) string {
	if p.Tenant == nil {
		return ""
	}
	for _, m := range p.Memberships {
		if m.TenantID == p.Tenant.ID {
			return m.Role
		}
	}
	return ""
}
func isManager(role string) bool { return role == "owner" || role == "admin" }
func isEditor(role string) bool  { return isManager(role) || role == "contributor" }
func statusOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	var u *weknora.Error
	if errors.As(err, &u) {
		return u.StatusCode
	}
	var h interface{ HTTPStatus() int }
	if errors.As(err, &h) {
		return h.HTTPStatus()
	}
	return 503
}
func (s *Service) principal(ctx context.Context, h http.Header, human bool) (weknora.Principal, error) {
	if s.Upstream == nil || s.Store == nil {
		return weknora.Principal{}, fail(503, "unavailable")
	}
	if human && (h.Get("X-API-Key") != "" || !strings.HasPrefix(h.Get("Authorization"), "Bearer ")) {
		return weknora.Principal{}, fail(403, "human_required")
	}
	p, err := s.Upstream.CurrentPrincipal(ctx, h)
	if err != nil {
		return p, fail(statusOf(err), "authentication_failed")
	}
	if p.Tenant == nil || p.Tenant.ID == 0 {
		return p, fail(403, "workspace_required")
	}
	if h.Get("X-API-Key") != "" {
		return p, nil
	}
	if p.User == nil || !p.User.IsActive || !validID(p.User.ID) {
		return p, fail(403, "identity_denied")
	}
	role := nativeRole(p)
	if !isEditor(role) && role != "viewer" {
		return p, fail(403, "membership_required")
	}
	if s.IdentityAllowed == nil {
		return p, fail(503, "identity_unavailable")
	}
	if err := s.IdentityAllowed(ctx, p.User.ID); err != nil {
		return p, fail(statusOf(err), "identity_denied")
	}
	return p, nil
}
func (s *Service) knowledgeBase(ctx context.Context, h http.Header, tenant uint64, id string) (kbIdentity, error) {
	var envelope struct {
		Success bool       `json:"success"`
		Data    kbIdentity `json:"data"`
	}
	if !validID(id) {
		return envelope.Data, fail(404, "not_found")
	}
	raw, err := s.Upstream.NativeRequest(ctx, http.MethodGet, "/api/v1/knowledge-bases/"+id, nil, h, nil)
	if err != nil {
		status := statusOf(err)
		if status == 403 || status == 404 {
			return envelope.Data, fail(404, "not_found")
		}
		return envelope.Data, fail(status, "upstream_unavailable")
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.Success || envelope.Data.ID != id {
		return envelope.Data, fail(502, "upstream_invalid")
	}
	if envelope.Data.TenantID != tenant || envelope.Data.IsTemporary {
		return envelope.Data, fail(404, "not_found")
	}
	return envelope.Data, nil
}
func (s *Service) ensure(ctx context.Context, kb kbIdentity, newlyCreated bool) (Policy, error) {
	p, err := s.Store.Policy(ctx, kb.TenantID, kb.ID)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	creator := kb.CreatorID
	if !validID(creator) {
		creator = ""
	}
	g, err := s.Store.Generation(ctx, kb.TenantID, creator)
	if err != nil {
		return p, err
	}
	return s.Store.Ensure(ctx, Policy{TenantID: kb.TenantID, KBID: kb.ID, CreatorUserID: creator, CreatorGeneration: g, CreatorRevoked: !newlyCreated && (g.Member > 0 || g.Identity > 0), Visibility: "restricted", Revision: 1, Members: []Member{}})
}

// EnsurePolicy initializes verified native metadata without widening visibility.
func (s *Service) EnsurePolicy(ctx context.Context, h http.Header, kbID string) (Policy, error) {
	p, err := s.principal(ctx, h, false)
	if err != nil {
		return Policy{}, err
	}
	kb, err := s.knowledgeBase(ctx, h, p.Tenant.ID, kbID)
	if err != nil {
		return Policy{}, err
	}
	return s.ensure(ctx, kb, false)
}
func (s *Service) humanPermissions(ctx context.Context, p weknora.Principal, policy Policy) (Permissions, error) {
	result := Permissions{Visibility: policy.Visibility, Revision: policy.Revision, Operations: []string{}}
	if policy.Deleted {
		return result, fail(404, "not_found")
	}
	role := nativeRole(p)
	if isManager(role) {
		result.Visible = true
		result.Manage = true
		result.Operations = append([]string{}, Operations...)
	}
	g, err := s.Store.Generation(ctx, policy.TenantID, p.User.ID)
	if err != nil {
		return result, err
	}
	result.Creator = policy.CreatorUserID == p.User.ID && !policy.CreatorRevoked && policy.CreatorGeneration == g
	if result.Creator {
		result.Visible = true
		if isEditor(role) {
			result.Manage = true
			result.Operations = append([]string{}, Operations...)
		}
	}
	if policy.Visibility == "workspace" {
		result.Visible = true
	}
	for _, m := range policy.Members {
		if m.UserID == p.User.ID && m.Generation == g && m.Visible {
			result.Visible = true
			if role == "contributor" && !result.Manage {
				result.Operations = append([]string{}, m.Operations...)
			}
		}
	}
	return result, nil
}
func (s *Service) Authorize(ctx context.Context, h http.Header, kbID string, actions ...string) (Permissions, error) {
	p, err := s.principal(ctx, h, false)
	if err != nil {
		return Permissions{}, err
	}
	kb, err := s.knowledgeBase(ctx, h, p.Tenant.ID, kbID)
	if err != nil {
		return Permissions{}, err
	}
	pending, err := s.Store.CreationPending(ctx, kb.TenantID, kb.CreatorID, kb.ID)
	if err != nil {
		return Permissions{}, err
	}
	// Administrators can inspect and recover an uncertain native result. Neither
	// their inspection/ACL edits nor lazy migration make it usable by others.
	if pending && (h.Get("X-API-Key") != "" || !isManager(nativeRole(p))) {
		return Permissions{}, fail(404, "not_found")
	}
	policy, err := s.ensure(ctx, kb, false)
	if err != nil {
		return Permissions{}, err
	}
	if policy.Deleted {
		return Permissions{}, fail(404, "not_found")
	}
	var result Permissions
	if h.Get("X-API-Key") != "" {
		if s.MachinePermissions == nil {
			return result, fail(404, "not_found")
		}
		result, err = s.MachinePermissions(ctx, h, kbID)
		// A machine scope is independent of projected native Owner identity.
		result.Manage = false
		result.Creator = false
		result.Visibility = policy.Visibility
		result.Revision = policy.Revision
	} else {
		result, err = s.humanPermissions(ctx, p, policy)
	}
	if err != nil {
		return Permissions{}, err
	}
	if !result.Visible {
		return Permissions{}, fail(404, "not_found")
	}
	for _, a := range actions {
		if a == "manage" {
			if !result.Manage {
				return result, fail(403, "operation_denied")
			}
			continue
		}
		if !contains(Operations, a) {
			return result, fail(403, "unknown_operation")
		}
		if !contains(result.Operations, a) {
			return result, fail(403, "operation_denied")
		}
	}
	if result.Operations == nil {
		result.Operations = []string{}
	}
	return result, nil
}
func (s *Service) Effective(ctx context.Context, h http.Header, kb string) (Permissions, error) {
	return s.Authorize(ctx, h, kb)
}
func (s *Service) AllowedKnowledgeBases(ctx context.Context, h http.Header, ids []string) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := s.Authorize(ctx, h, id); err != nil {
			if statusOf(err) == 404 {
				continue
			}
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func (s *Service) roster(ctx context.Context, h http.Header, tenant uint64) ([]weknora.TenantMember, error) {
	if s.Members != nil {
		return s.Members(ctx, h, tenant)
	}
	u, ok := s.Upstream.(interface {
		EnterpriseRequest(context.Context, string, string, http.Header, any) (json.RawMessage, error)
	})
	if !ok {
		return nil, fail(503, "members_unavailable")
	}
	result := []weknora.TenantMember{}
	for page := 1; page <= 100; page++ {
		raw, err := u.EnterpriseRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d/members?page=%d&page_size=100", tenant, page), h, nil)
		if err != nil {
			return nil, err
		}
		var e struct {
			Success bool `json:"success"`
			Data    struct {
				Members []weknora.TenantMember `json:"members"`
				Total   int                    `json:"total"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &e) != nil || !e.Success || e.Data.Total < 0 {
			return nil, fail(502, "members_invalid")
		}
		result = append(result, e.Data.Members...)
		if len(result) >= e.Data.Total {
			return result, nil
		}
		if len(e.Data.Members) == 0 {
			return nil, fail(502, "members_invalid")
		}
	}
	return nil, fail(413, "members_limit")
}
func (s *Service) managerPolicy(ctx context.Context, h http.Header, kb string) (weknora.Principal, Policy, error) {
	p, err := s.principal(ctx, h, true)
	if err != nil {
		return p, Policy{}, err
	}
	if _, err = s.Authorize(ctx, h, kb, "manage"); err != nil {
		return p, Policy{}, err
	}
	policy, err := s.Store.Policy(ctx, p.Tenant.ID, kb)
	return p, policy, err
}
func event(ctx context.Context, p weknora.Principal, before Policy, action, target string) Event {
	raw, _ := json.Marshal(before)
	return Event{TenantID: before.TenantID, KBID: before.KBID, ActorUserID: p.User.ID, Action: action, Target: target, Before: raw, Outcome: "success", RequestID: diagnostics.RequestID(ctx)}
}
func (s *Service) locked(ctx context.Context, h http.Header, kb string, fn func() error) error {
	if s.WithAuthorizationLock == nil {
		return fail(503, "lock_unavailable")
	}
	return s.WithAuthorizationLock(ctx, h, []string{kb}, fn)
}

type PolicyInput struct {
	Revision   uint64   `json:"revision"`
	Visibility string   `json:"visibility"`
	Members    []Member `json:"members"`
}

func (s *Service) UpdatePolicy(ctx context.Context, h http.Header, kb string, input PolicyInput) (Policy, error) {
	var result Policy
	err := s.locked(ctx, h, kb, func() error {
		p, before, err := s.managerPolicy(ctx, h, kb)
		if err != nil {
			return err
		}
		if input.Revision == 0 || input.Revision != before.Revision {
			return fail(409, "revision_conflict")
		}
		if input.Visibility != "restricted" && input.Visibility != "workspace" || input.Members == nil || len(input.Members) > 1000 {
			return fail(400, "policy_invalid")
		}
		roster, err := s.roster(ctx, h, p.Tenant.ID)
		if err != nil {
			return err
		}
		roles := map[string]string{}
		for _, m := range roster {
			if m.Status == "active" {
				roles[m.UserID] = m.Role
			}
		}
		next := before
		next.Visibility = input.Visibility
		next.Members = []Member{}
		seen := map[string]bool{}
		for _, m := range input.Members {
			if !validID(m.UserID) || seen[m.UserID] || roles[m.UserID] == "" {
				return fail(400, "member_invalid")
			}
			seen[m.UserID] = true
			// Snapshot before the identity check. A concurrent suspend/rejoin
			// increments the generation, so a late save cannot revive a grant.
			m.Generation, err = s.Store.Generation(ctx, p.Tenant.ID, m.UserID)
			if err != nil {
				return err
			}
			if err = s.IdentityAllowed(ctx, m.UserID); err != nil {
				return fail(statusOf(err), "member_unavailable")
			}
			if len(m.Operations) > 0 && (!m.Visible || roles[m.UserID] != "contributor") {
				return fail(400, "member_operations_invalid")
			}
			ops := map[string]bool{}
			for _, op := range m.Operations {
				if !contains(Operations, op) || ops[op] {
					return fail(400, "operation_invalid")
				}
				ops[op] = true
			}
			if !m.Visible {
				continue
			}
			if m.Operations == nil {
				m.Operations = []string{}
			}
			sort.Strings(m.Operations)
			next.Members = append(next.Members, m)
		}
		sort.Slice(next.Members, func(i, j int) bool { return next.Members[i].UserID < next.Members[j].UserID })
		result, err = s.Store.Save(ctx, next, input.Revision, event(ctx, p, before, "policy.update", ""))
		return translateStore(err)
	})
	return result, err
}
func translateStore(err error) error {
	if errors.Is(err, ErrConflict) {
		return fail(409, "revision_conflict")
	}
	return err
}

// BindCreated is called only by the guarded create response while its
// authorization locks remain held. Client-supplied creator fields are ignored.
func (s *Service) BindCreated(ctx context.Context, h http.Header, kb string) error {
	p, err := s.principal(ctx, h, false)
	if err != nil {
		return err
	}
	native, err := s.knowledgeBase(ctx, h, p.Tenant.ID, kb)
	if err != nil {
		return err
	}
	verifiedHuman := h.Get("X-API-Key") == "" && native.CreatorID == p.User.ID && isEditor(nativeRole(p))
	if h.Get("X-API-Key") == "" && !verifiedHuman {
		return fail(409, "creator_mismatch")
	}
	if h.Get("X-API-Key") != "" {
		native.CreatorID = ""
	}
	policy, err := s.ensure(ctx, native, verifiedHuman)
	if err == nil && policy.Deleted {
		return fail(404, "not_found")
	}
	return err
}
func (s *Service) BindCreatedLocked(ctx context.Context, h http.Header, kb string) error {
	return s.BindCreated(ctx, h, kb)
}

// MarkDeleted follows successful native deletion; the native object can no
// longer be fetched, so the already-established policy supplies its boundary.
func (s *Service) MarkDeleted(ctx context.Context, h http.Header, kb string) error {
	p, err := s.principal(ctx, h, true)
	if err != nil {
		return err
	}
	policy, err := s.Store.Policy(ctx, p.Tenant.ID, kb)
	if err != nil {
		return err
	}
	perms, err := s.humanPermissions(ctx, p, policy)
	if err != nil {
		return err
	}
	if !perms.Manage {
		return fail(403, "operation_denied")
	}
	next := policy
	next.Deleted = true
	next.Members = []Member{}
	_, err = s.Store.Save(ctx, next, policy.Revision, event(ctx, p, policy, "policy.delete", ""))
	return translateStore(err)
}
func (s *Service) MarkDeletedLocked(ctx context.Context, h http.Header, kb string) error {
	return s.MarkDeleted(ctx, h, kb)
}
func (s *Service) RestoreCreator(ctx context.Context, h http.Header, kb string, revision uint64) (Policy, error) {
	var result Policy
	err := s.locked(ctx, h, kb, func() error {
		p, before, err := s.managerPolicy(ctx, h, kb)
		if err != nil {
			return err
		}
		if !isManager(nativeRole(p)) {
			return fail(403, "admin_required")
		}
		if revision == 0 || revision != before.Revision {
			return fail(409, "revision_conflict")
		}
		if before.CreatorUserID == "" {
			return fail(409, "creator_unknown")
		}
		g, err := s.Store.Generation(ctx, p.Tenant.ID, before.CreatorUserID)
		if err != nil {
			return err
		}
		roster, err := s.roster(ctx, h, p.Tenant.ID)
		if err != nil {
			return err
		}
		active := false
		for _, m := range roster {
			if m.UserID == before.CreatorUserID && m.Status == "active" && isEditor(m.Role) {
				active = true
			}
		}
		if !active {
			return fail(409, "creator_unavailable")
		}
		if err = s.IdentityAllowed(ctx, before.CreatorUserID); err != nil {
			return fail(statusOf(err), "creator_unavailable")
		}
		next := before
		next.CreatorRevoked = false
		next.CreatorGeneration = g
		result, err = s.Store.Save(ctx, next, revision, event(ctx, p, before, "creator.restore", before.CreatorUserID))
		return translateStore(err)
	})
	return result, err
}
func (s *Service) RevokeMember(ctx context.Context, tenant uint64, user string, remove bool) error {
	if tenant == 0 || !validID(user) {
		return fail(400, "member_invalid")
	}
	return s.Store.RevokeMember(ctx, tenant, user, remove)
}
func (s *Service) RevokeIdentity(ctx context.Context, user string) error {
	if !validID(user) {
		return fail(400, "member_invalid")
	}
	return s.Store.RevokeIdentity(ctx, user)
}
