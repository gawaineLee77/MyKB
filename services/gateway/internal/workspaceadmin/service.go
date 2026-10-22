package workspaceadmin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string              { return e.Code }
func (e *Error) HTTPStatus() int            { return e.Status }
func (e *Error) PublicCode() string         { return e.Code }
func failure(code string, status int) error { return &Error{Code: code, Status: status} }
func PublicError(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "workspace_admin.operation_failed"
}

type Service struct {
	Store                 Store
	KnowledgeStore        KnowledgeStore
	Upstream              enterprise.Upstream
	IdentityAllowed       func(context.Context, string) error
	ValidateKeyScope      func(context.Context, weknora.Principal, http.Header, []string) error
	ProtectedKey          func(context.Context, uint64, uint64) (bool, error)
	RevokeKnowledgeMember func(context.Context, uint64, string, bool, string) error
	aead                  cipher.AEAD
}

func NewService(store Store, upstream enterprise.Upstream, secret []byte) (*Service, error) {
	if store == nil || upstream == nil {
		return nil, fmt.Errorf("workspace administration requires store and upstream")
	}
	s := &Service{Store: store, Upstream: upstream}
	if len(secret) > 0 {
		if len(secret) != 32 {
			return nil, fmt.Errorf("workspace administration encryption key must be 32 bytes")
		}
		block, err := aes.NewCipher(secret)
		if err != nil {
			return nil, err
		}
		s.aead, err = cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}
func tenantPath(id uint64) string { return "/api/v1/tenants/" + strconv.FormatUint(id, 10) }
func role(p weknora.Principal, id uint64) string {
	for _, m := range p.Memberships {
		if m.TenantID == id {
			return m.Role
		}
	}
	return ""
}
func (s *Service) withSpace(ctx context.Context, id uint64, fn func(Store) error) error {
	if id == 0 {
		return failure("workspace_admin.tenant_required", 400)
	}
	return s.Store.WithLock(ctx, "workspace-admin:"+strconv.FormatUint(id, 10), fn)
}
func (s *Service) freshPrincipal(ctx context.Context, p weknora.Principal, h http.Header, id uint64) (weknora.Principal, error) {
	if !strings.HasPrefix(h.Get("Authorization"), "Bearer ") || h.Get("X-API-Key") != "" {
		return weknora.Principal{}, failure("workspace_admin.human_required", 403)
	}
	fresh, err := s.Upstream.CurrentPrincipal(ctx, h)
	if err != nil {
		return fresh, failure("workspace_admin.authentication_failed", 401)
	}
	if fresh.User == nil || !fresh.User.IsActive || p.User == nil || fresh.User.ID != p.User.ID || fresh.Tenant == nil || fresh.Tenant.ID != id {
		return fresh, failure("workspace_admin.access_denied", 403)
	}
	if r := role(fresh, id); r != "owner" && r != "admin" {
		return fresh, failure("workspace_admin.admin_required", 403)
	}
	if s.IdentityAllowed == nil {
		return fresh, failure("workspace_admin.identity_unavailable", 503)
	}
	if err := s.IdentityAllowed(ctx, fresh.User.ID); err != nil {
		return fresh, identityFailure(err)
	}
	return fresh, nil
}
func (s *Service) audit(ctx context.Context, store Store, id uint64, user, action, target, outcome string) error {
	return store.RecordAudit(ctx, Audit{TenantID: id, ActorUserID: user, Action: action, Target: target, Outcome: outcome, CorrelationID: diagnostics.RequestID(ctx)})
}
func (s *Service) seal(id uint64, label string, plain []byte) ([]byte, error) {
	if s.aead == nil {
		return nil, failure("workspace_admin.delegation_not_configured", 503)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plain, []byte(fmt.Sprintf("workspace-admin:%d:%s", id, label))), nil
}
func (s *Service) open(id uint64, label string, sealed []byte) ([]byte, error) {
	if s.aead == nil || len(sealed) < s.aead.NonceSize() {
		return nil, failure("workspace_admin.delegation_unavailable", 503)
	}
	n := s.aead.NonceSize()
	plain, err := s.aead.Open(nil, sealed[:n], sealed[n:], []byte(fmt.Sprintf("workspace-admin:%d:%s", id, label)))
	if err != nil {
		return nil, failure("workspace_admin.delegation_unavailable", 503)
	}
	return plain, nil
}
func fingerprint(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func (s *Service) DisableCreatorKeys(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return failure("workspace_admin.user_required", 400)
	}
	return s.Store.DisableUserKeys(ctx, 0, userID)
}

// No delegated credentials are exposed in these responses.
type DelegationInput struct{}
type DelegationStatus struct {
	Configured  bool       `json:"configured"`
	Authorized  bool       `json:"authorized"`
	OwnerUserID string     `json:"owner_user_id,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	ErrorCode   string     `json:"error_code,omitempty"`
}
type ownerSession struct {
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}
type sessionResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

func parseSession(raw json.RawMessage) (ownerSession, error) {
	var r sessionResponse
	if json.Unmarshal(raw, &r) != nil || !r.Success {
		return ownerSession{}, failure("workspace_admin.delegation_invalid_response", 502)
	}
	if r.Access == "" {
		r.Access = r.Token
	}
	if r.Access == "" || r.Refresh == "" {
		return ownerSession{}, failure("workspace_admin.delegation_invalid_response", 502)
	}
	return ownerSession{Access: r.Access, Refresh: r.Refresh}, nil
}
func ownerHeader(token string, id uint64) http.Header {
	h := enterprise.Bearer(token)
	h.Set("X-Tenant-ID", strconv.FormatUint(id, 10))
	return h
}
func (s *Service) checkOwner(ctx context.Context, owner string, id uint64, h http.Header) error {
	p, err := s.Upstream.CurrentPrincipal(ctx, h)
	if err != nil {
		return err
	}
	if p.User == nil || !p.User.IsActive || p.User.ID != owner || p.Tenant == nil || p.Tenant.ID != id || role(p, id) != "owner" {
		return failure("workspace_admin.delegation_owner_invalid", 403)
	}
	if s.IdentityAllowed == nil {
		return failure("workspace_admin.identity_unavailable", 503)
	}
	if err := s.IdentityAllowed(ctx, owner); err != nil {
		return identityFailure(err)
	}
	return nil
}
func (s *Service) SetDelegation(ctx context.Context, p weknora.Principal, h http.Header, id uint64, _ DelegationInput) (DelegationStatus, error) {
	result := DelegationStatus{Configured: s.aead != nil}
	err := s.withSpace(ctx, id, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, id)
		if err != nil {
			return err
		}
		if role(fresh, id) != "owner" {
			return failure("workspace_admin.owner_required", 403)
		}
		if s.aead == nil {
			return failure("workspace_admin.delegation_not_configured", 503)
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "delegation.authorize", "", "attempt"); err != nil {
			return err
		}
		raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/switch-tenant", h, map[string]uint64{"tenant_id": id})
		if err != nil {
			return failure("workspace_admin.delegation_unavailable", 503)
		}
		session, err := parseSession(raw)
		if err != nil {
			return err
		}
		// The pinned signer has second-resolution JWTs and no nonce. Never adopt
		// the browser's exact session when authorization occurs in its mint second.
		if "Bearer "+session.Access == h.Get("Authorization") {
			return failure("workspace_admin.delegation_retry", 409)
		}
		if err := s.checkOwner(ctx, fresh.User.ID, id, ownerHeader(session.Access, id)); err != nil {
			return identityFailure(err)
		}
		plain, _ := json.Marshal(session)
		sealed, err := s.seal(id, "session", plain)
		if err != nil {
			return err
		}
		d, err := store.Delegation(ctx, id)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		d.TenantID = id
		d.OwnerUserID = fresh.User.ID
		d.SessionCipher = sealed
		if err := store.SaveDelegation(ctx, d); err != nil {
			return err
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "delegation.authorize", "", "success"); err != nil {
			return err
		}
		now := time.Now().UTC()
		result.Authorized = true
		result.OwnerUserID = fresh.User.ID
		result.UpdatedAt = &now
		return nil
	})
	return result, err
}
func (s *Service) GetDelegationStatus(ctx context.Context, p weknora.Principal, h http.Header, id uint64) (DelegationStatus, error) {
	result := DelegationStatus{Configured: s.aead != nil}
	err := s.withSpace(ctx, id, func(store Store) error {
		if _, err := s.freshPrincipal(ctx, p, h, id); err != nil {
			return err
		}
		d, err := store.Delegation(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result.OwnerUserID = d.OwnerUserID
		result.UpdatedAt = &d.UpdatedAt
		if _, err := s.delegatedHeaders(ctx, store, id); err != nil {
			result.ErrorCode = PublicError(err)
			return nil
		}
		result.Authorized = true
		return nil
	})
	return result, err
}
func (s *Service) RevokeDelegation(ctx context.Context, p weknora.Principal, h http.Header, id uint64) error {
	return s.withSpace(ctx, id, func(store Store) error {
		fresh, err := s.freshPrincipal(ctx, p, h, id)
		if err != nil {
			return err
		}
		if role(fresh, id) != "owner" {
			return failure("workspace_admin.owner_required", 403)
		}
		if err := s.audit(ctx, store, id, fresh.User.ID, "delegation.revoke", "", "attempt"); err != nil {
			return err
		}
		d, err := store.Delegation(ctx, id)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		// Disable local delegation before any remote revocation. An upstream
		// outage must never keep a supposedly revoked Owner proxy usable.
		// Retain the internal key ID for a retry if native revocation fails.
		if err == nil {
			d.SessionCipher = []byte{}
			if err := store.SaveDelegation(ctx, d); err != nil {
				return err
			}
		}
		// Native logout revokes all Owner sessions. Forget the delegated session
		// locally; revoke only its machine credential through the key API.
		if d.MemberKeyID != 0 {
			if _, err := s.Upstream.EnterpriseRequest(ctx, "DELETE", tenantPath(id)+"/api-keys/"+strconv.FormatUint(d.MemberKeyID, 10), h, nil); err != nil {
				return failure("workspace_admin.delegation_revoke_failed", 503)
			}
		}
		if err := store.DeleteDelegation(ctx, id); err != nil {
			return err
		}
		return s.audit(ctx, store, id, fresh.User.ID, "delegation.revoke", "", "success")
	})
}
func (s *Service) ownerHeaders(ctx context.Context, store Store, p weknora.Principal, h http.Header, id uint64) (http.Header, error) {
	if role(p, id) == "owner" {
		return ownerHeader(strings.TrimPrefix(h.Get("Authorization"), "Bearer "), id), nil
	}
	return s.delegatedHeaders(ctx, store, id)
}
func (s *Service) delegatedHeaders(ctx context.Context, store Store, id uint64) (http.Header, error) {
	return s.delegatedHeadersFor(ctx, store, id, "session")
}
func (s *Service) delegatedHeadersFor(ctx context.Context, store Store, id uint64, purpose string) (http.Header, error) {
	return s.validatedDelegatedHeaders(ctx, store, id, purpose, true)
}

// Refresh writes the delegation and is only safe under its workspace lock.
// Retrieval checks can overlap revocation, so they only validate the existing
// access token; the gateway's initial CheckKeyUse performs locked refresh.
func (s *Service) validatedDelegatedHeaders(ctx context.Context, store Store, id uint64, purpose string, allowRefresh bool) (http.Header, error) {
	d, err := store.Delegation(ctx, id)
	if err != nil {
		return nil, failure("workspace_admin.delegation_required", 503)
	}
	plain, err := s.open(id, purpose, d.SessionCipher)
	if err != nil {
		return nil, err
	}
	var session ownerSession
	if json.Unmarshal(plain, &session) != nil || session.Access == "" || session.Refresh == "" {
		return nil, failure("workspace_admin.delegation_unavailable", 503)
	}
	h := ownerHeader(session.Access, id)
	err = s.checkOwner(ctx, d.OwnerUserID, id, h)
	if err == nil {
		return h, nil
	}
	var upstreamErr *weknora.Error
	if !errors.As(err, &upstreamErr) || upstreamErr.UpstreamStatus != 401 {
		return nil, identityFailure(err)
	}
	if !allowRefresh {
		return nil, failure("workspace_admin.delegation_expired", 503)
	}
	raw, err := s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/refresh", nil, map[string]string{"refreshToken": session.Refresh})
	if err != nil {
		return nil, failure("workspace_admin.delegation_expired", 503)
	}
	session, err = parseSession(raw)
	if err != nil {
		return nil, err
	}
	// Refresh follows the owner's last active workspace. Rebind without revoking
	// the just-refreshed token: a same-second refresh and switch can share it.
	refreshed, err := s.Upstream.CurrentPrincipal(ctx, ownerHeader(session.Access, id))
	if err != nil {
		return nil, failure("workspace_admin.delegation_unavailable", 503)
	}
	if refreshed.User == nil || refreshed.User.ID != d.OwnerUserID || !refreshed.User.IsActive || role(refreshed, id) != "owner" {
		return nil, failure("workspace_admin.delegation_owner_invalid", 403)
	}
	if refreshed.Tenant == nil || refreshed.Tenant.ID != id {
		raw, err = s.Upstream.EnterpriseRequest(ctx, "POST", "/api/v1/auth/switch-tenant", ownerHeader(session.Access, id), map[string]uint64{"tenant_id": id})
		if err != nil {
			return nil, failure("workspace_admin.delegation_expired", 503)
		}
		session, err = parseSession(raw)
		if err != nil {
			return nil, err
		}
	}
	h = ownerHeader(session.Access, id)
	if err := s.checkOwner(ctx, d.OwnerUserID, id, h); err != nil {
		return nil, identityFailure(err)
	}
	plain, _ = json.Marshal(session)
	d.SessionCipher, err = s.seal(id, purpose, plain)
	if err != nil {
		return nil, err
	}
	if err := store.SaveDelegation(ctx, d); err != nil {
		return nil, err
	}
	return h, nil
}

func identityFailure(err error) error {
	var denied *Error
	if errors.As(err, &denied) && denied.Status == 403 {
		return failure("workspace_admin.identity_denied", 403)
	}
	return failure("workspace_admin.identity_unavailable", 503)
}
