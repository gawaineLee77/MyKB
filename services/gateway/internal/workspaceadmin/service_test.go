package workspaceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type memoryStore struct {
	delegations map[uint64]Delegation
	keys        map[string]KeyOwner
	pending     map[uint64]map[string]uint64
	generations map[string]uint64
	audits      []Audit
	failSaveKey bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{delegations: map[uint64]Delegation{}, keys: map[string]KeyOwner{}, pending: map[uint64]map[string]uint64{}, generations: map[string]uint64{}}
}
func keyRef(id, key uint64) string { return fmt.Sprintf("%d:%d", id, key) }
func (m *memoryStore) WithLock(ctx context.Context, _ string, fn func(Store) error) error {
	return fn(m)
}
func (m *memoryStore) Delegation(_ context.Context, id uint64) (Delegation, error) {
	d, ok := m.delegations[id]
	if !ok {
		return d, ErrNotFound
	}
	return d, nil
}
func (m *memoryStore) SaveDelegation(_ context.Context, d Delegation) error {
	m.delegations[d.TenantID] = d
	return nil
}
func (m *memoryStore) DeleteDelegation(_ context.Context, id uint64) error {
	delete(m.delegations, id)
	return nil
}
func (m *memoryStore) Key(_ context.Context, id, key uint64) (KeyOwner, error) {
	k, ok := m.keys[keyRef(id, key)]
	if !ok {
		return k, ErrNotFound
	}
	return k, nil
}
func (m *memoryStore) KeyByFingerprint(_ context.Context, f string) (KeyOwner, error) {
	for _, k := range m.keys {
		if k.Fingerprint == f {
			return k, nil
		}
	}
	return KeyOwner{}, ErrNotFound
}
func (m *memoryStore) SaveKey(_ context.Context, k KeyOwner) error {
	if m.failSaveKey {
		return errors.New("metadata unavailable")
	}
	m.keys[keyRef(k.TenantID, k.KeyID)] = k
	return nil
}
func (m *memoryStore) DisableKey(_ context.Context, id, key uint64) error {
	k, ok := m.keys[keyRef(id, key)]
	if ok {
		k.Disabled = true
		m.keys[keyRef(id, key)] = k
	}
	return nil
}
func (m *memoryStore) DisableUserKeys(ctx context.Context, id uint64, user string) error {
	if id == 0 {
		m.generations[user]++
	}
	for _, k := range m.keys {
		if (id == 0 || k.TenantID == id) && k.CreatorUserID == user {
			_ = m.DisableKey(ctx, k.TenantID, k.KeyID)
		}
	}
	return nil
}
func (m *memoryStore) CreatorGeneration(_ context.Context, user string) (uint64, error) {
	return m.generations[user], nil
}
func (m *memoryStore) PendingCreates(_ context.Context, id uint64) ([]PendingCreate, error) {
	result := []PendingCreate{}
	for marker, key := range m.pending[id] {
		result = append(result, PendingCreate{Marker: marker, KeyID: key})
	}
	return result, nil
}
func (m *memoryStore) SavePending(_ context.Context, id uint64, marker string, key uint64) error {
	if m.pending[id] == nil {
		m.pending[id] = map[string]uint64{}
	}
	m.pending[id][marker] = key
	return nil
}
func (m *memoryStore) DeletePending(_ context.Context, id uint64, marker string) error {
	delete(m.pending[id], marker)
	return nil
}
func (m *memoryStore) RecordAudit(_ context.Context, a Audit) error {
	m.audits = append(m.audits, a)
	return nil
}

type fakeUpstream struct {
	principals                 map[string]weknora.Principal
	principalErrors            map[string]error
	keys                       map[uint64]keyDTO
	members                    []weknora.TenantMember
	calls                      []string
	next                       uint64
	lostCreate, failDelete     bool
	onCreate                   func()
	createResponse             func(*keyDTO)
	mintedToken, refreshAccess string
}

func principal(user, role string, id uint64) weknora.Principal {
	return weknora.Principal{User: &weknora.User{ID: user, IsActive: true}, Tenant: &weknora.Tenant{ID: id}, Memberships: []weknora.Membership{{TenantID: id, Role: role}}}
}
func (f *fakeUpstream) CurrentPrincipal(_ context.Context, h http.Header) (weknora.Principal, error) {
	token := strings.TrimPrefix(h.Get("Authorization"), "Bearer ")
	if err := f.principalErrors[token]; err != nil {
		return weknora.Principal{}, err
	}
	p, ok := f.principals[token]
	if !ok {
		return p, &weknora.Error{UpstreamStatus: 401}
	}
	return p, nil
}
func encode(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
func (f *fakeUpstream) EnterpriseRequest(_ context.Context, method, path string, h http.Header, input any) (json.RawMessage, error) {
	f.calls = append(f.calls, method+" "+path)
	if path == "/api/v1/auth/switch-tenant" {
		token := f.mintedToken
		if token == "" {
			token = "delegated"
		}
		return encode(map[string]any{"success": true, "token": token, "refresh_token": "server-refresh"}), nil
	}
	if path == "/api/v1/auth/refresh" {
		return encode(map[string]any{"success": true, "access_token": f.refreshAccess, "refresh_token": "rotated-refresh"}), nil
	}
	if strings.Contains(path, "/members?") {
		return encode(map[string]any{"success": true, "data": map[string]any{"members": f.members, "total": len(f.members)}}), nil
	}
	if !strings.Contains(path, "/api-keys") {
		return nil, fmt.Errorf("unexpected request")
	}
	switch method {
	case "GET":
		keys := []keyDTO{}
		for _, k := range f.keys {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
		return encode(map[string]any{"success": true, "data": keys}), nil
	case "POST":
		var req keyInput
		_ = json.Unmarshal(encode(input), &req)
		f.next++
		k := keyDTO{ID: f.next, ScopeType: "tenant", Name: req.Name, APIKey: fmt.Sprintf("secret-%d", f.next), Token: fmt.Sprintf("secret-%d", f.next), FullAccess: req.FullAccess, Capabilities: req.Capabilities, KnowledgeBaseIDs: req.KnowledgeBaseIDs}
		f.keys[k.ID] = k
		if f.onCreate != nil {
			f.onCreate()
		}
		if f.lostCreate {
			return nil, errors.New("response lost")
		}
		if f.createResponse != nil {
			f.createResponse(&k)
		}
		return encode(map[string]any{"success": true, "data": k}), nil
	case "PUT":
		id, _ := strconv.ParseUint(path[strings.LastIndex(path, "/")+1:], 10, 64)
		k, ok := f.keys[id]
		if !ok {
			return nil, errors.New("not found")
		}
		var req keyInput
		_ = json.Unmarshal(encode(input), &req)
		k.Name = req.Name
		k.FullAccess = req.FullAccess
		k.KnowledgeBaseIDs = req.KnowledgeBaseIDs
		k.Capabilities = req.Capabilities
		f.keys[id] = k
		return encode(map[string]any{"success": true, "data": k}), nil
	case "DELETE":
		if f.failDelete {
			return nil, errors.New("unavailable")
		}
		id, _ := strconv.ParseUint(path[strings.LastIndex(path, "/")+1:], 10, 64)
		delete(f.keys, id)
		return encode(map[string]bool{"success": true}), nil
	}
	return nil, fmt.Errorf("unexpected method")
}
func fixture(t *testing.T) (*Service, *memoryStore, *fakeUpstream) {
	t.Helper()
	store := newMemoryStore()
	upstream := &fakeUpstream{principals: map[string]weknora.Principal{"owner": principal("o", "owner", 1), "admin": principal("a", "admin", 1), "other": principal("b", "admin", 1), "viewer": principal("v", "viewer", 1), "delegated": principal("o", "owner", 1)}, principalErrors: map[string]error{}, keys: map[uint64]keyDTO{}, members: []weknora.TenantMember{{UserID: "a", Role: "admin", Status: "active"}, {UserID: "o", Role: "owner", Status: "active"}}, next: 100}
	service, err := NewService(store, upstream, []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	service.IdentityAllowed = func(context.Context, string) error { return nil }
	service.ValidateKeyScope = func(context.Context, weknora.Principal, http.Header, []string) error { return nil }
	sealed, err := service.seal(1, "session", encode(ownerSession{Access: "delegated", Refresh: "server-refresh"}))
	if err != nil {
		t.Fatal(err)
	}
	store.delegations[1] = Delegation{TenantID: 1, OwnerUserID: "o", SessionCipher: sealed}
	return service, store, upstream
}
func header(token string) http.Header {
	return http.Header{"Authorization": []string{"Bearer " + token}}
}
func validInput() json.RawMessage {
	return json.RawMessage(`{"name":"Integration","full_access":false,"knowledge_base_ids":["kb-1"],"capabilities":["retrieve"]}`)
}
func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || PublicError(err) != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

func TestAdminKeyAttributionAndIsolation(t *testing.T) {
	s, store, u := fixture(t)
	ctx := diagnostics.WithRequestID(context.Background(), "request-1")
	raw, err := s.CreateKey(ctx, u.principals["admin"], header("admin"), 1, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"creator_user_id":"a"`) || !strings.Contains(string(raw), `"restricted":true`) {
		t.Fatalf("missing attribution: %s", raw)
	}
	record := store.keys["1:101"]
	if record.Fingerprint != fingerprint("secret-101") || record.CreatorUserID != "a" || record.RequiredRole != "admin" {
		t.Fatalf("incorrect metadata %#v", record)
	}
	if store.audits[0].ActorUserID != "a" || store.audits[0].CorrelationID != "request-1" {
		t.Fatalf("wrong actor audit %#v", store.audits)
	}
	u.keys[2] = keyDTO{ID: 2, ScopeType: "tenant", Name: "Legacy", APIKey: "legacy-secret"}
	list, err := s.ListKeys(ctx, u.principals["other"], header("other"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(list), "secret-") || strings.Contains(string(list), "legacy-secret") {
		t.Fatalf("leaked other secret: %s", list)
	}
	_, err = s.UpdateKey(ctx, u.principals["other"], header("other"), 1, 101, validInput())
	assertCode(t, err, "workspace_admin.key_not_found")
	err = s.RevokeKey(ctx, u.principals["other"], header("other"), 1, 101)
	assertCode(t, err, "workspace_admin.key_not_found")
	list, err = s.ListKeys(ctx, u.principals["owner"], header("owner"), 1)
	if err != nil || !strings.Contains(string(list), "legacy-secret") || !strings.Contains(string(list), "secret-101") {
		t.Fatalf("owner all list %s %v", list, err)
	}
}
func TestKeyRoleScopeAndUnknownFieldBoundaries(t *testing.T) {
	tests := []struct{ name, actor, input, code string }{
		{"viewer", "viewer", string(validInput()), "workspace_admin.admin_required"},
		{"full access", "admin", `{"name":"x","full_access":true}`, "workspace_admin.key_scope_required"},
		{"unbounded", "admin", `{"name":"x","capabilities":["retrieve"]}`, "workspace_admin.key_scope_required"},
		{"members", "admin", `{"name":"x","knowledge_base_ids":["kb"],"capabilities":["manage_members"]}`, "workspace_admin.key_capability_denied"},
		{"models", "admin", `{"name":"x","knowledge_base_ids":["kb"],"capabilities":["manage_models"]}`, "workspace_admin.key_capability_denied"},
		{"creator spoof", "admin", `{"name":"x","creator_user_id":"o"}`, "workspace_admin.key_input_invalid"},
		{"scope spoof", "admin", `{"name":"x","scope_type":"platform"}`, "workspace_admin.key_input_invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _, u := fixture(t)
			_, err := s.CreateKey(context.Background(), u.principals[tc.actor], header(tc.actor), 1, json.RawMessage(tc.input))
			assertCode(t, err, tc.code)
			if len(u.keys) != 0 {
				t.Fatal("denied operation created key")
			}
		})
	}
	s, _, u := fixture(t)
	s.ValidateKeyScope = func(context.Context, weknora.Principal, http.Header, []string) error {
		return failure("scope.denied", 403)
	}
	_, err := s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 1, validInput())
	assertCode(t, err, "scope.denied")
	_, err = s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 2, validInput())
	assertCode(t, err, "workspace_admin.access_denied")
}
func TestCreateRequiresDelegationEvenForOwner(t *testing.T) {
	s, store, u := fixture(t)
	delete(store.delegations, 1)
	_, err := s.CreateKey(context.Background(), u.principals["owner"], header("owner"), 1, validInput())
	assertCode(t, err, "workspace_admin.delegation_required")
	if len(u.keys) != 0 {
		t.Fatal("created unusable key")
	}
}
func TestLostCreateAndAttributionFailureRevokeBeforeReturning(t *testing.T) {
	for _, kind := range []string{"lost response", "metadata failure"} {
		t.Run(kind, func(t *testing.T) {
			s, store, u := fixture(t)
			u.lostCreate = kind == "lost response"
			store.failSaveKey = kind == "metadata failure"
			raw, err := s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 1, validInput())
			if err == nil || len(raw) != 0 || len(u.keys) != 0 || len(store.pending[1]) != 0 {
				t.Fatalf("orphan survived: raw=%s error=%v keys=%d pending=%d", raw, err, len(u.keys), len(store.pending[1]))
			}
		})
	}
}
func TestPendingRecoveryCannotLeakAsLegacy(t *testing.T) {
	s, store, u := fixture(t)
	u.keys[5] = keyDTO{ID: 5, ScopeType: "tenant", Name: "already renamed", APIKey: "orphan-secret"}
	store.pending[1] = map[string]uint64{pendingPrefix + "op": 5}
	u.failDelete = true
	raw, err := s.ListKeys(context.Background(), u.principals["owner"], header("owner"), 1)
	assertCode(t, err, "workspace_admin.key_recovery_required")
	if len(raw) != 0 {
		t.Fatal("returned orphan")
	}
	u.failDelete = false
	_, err = s.ListKeys(context.Background(), u.principals["owner"], header("owner"), 1)
	if err != nil || len(u.keys) != 0 {
		t.Fatal("recovery failed", err)
	}
}

func TestUnknownCreateMarkerSurvivesDelayedCommit(t *testing.T) {
	s, store, u := fixture(t)
	marker := pendingPrefix + "delayed"
	store.pending[1] = map[string]uint64{marker: 0}
	if _, err := s.ListKeys(context.Background(), u.principals["owner"], header("owner"), 1); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.pending[1][marker]; !exists {
		t.Fatal("absence was incorrectly treated as proof the create cannot commit")
	}
	u.keys[8] = keyDTO{ID: 8, ScopeType: "tenant", Name: marker, APIKey: "late-secret"}
	raw, err := s.ListKeys(context.Background(), u.principals["owner"], header("owner"), 1)
	if err != nil || strings.Contains(string(raw), "late-secret") || len(u.keys) != 0 || len(store.pending[1]) != 0 {
		t.Fatalf("late key not recovered safely: %s %v", raw, err)
	}
}
func TestRuntimeCreatorRevocationPersistsAfterPromotion(t *testing.T) {
	s, store, u := fixture(t)
	_, err := s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 1, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckKeyUse(context.Background(), "secret-101", 0); err != nil {
		t.Fatal(err)
	}
	assertCode(t, s.CheckKeyUse(context.Background(), "secret-101", 2), "workspace_admin.key_tenant_denied")
	u.members[0].Role = "viewer"
	assertCode(t, s.CheckKeyUse(context.Background(), "secret-101", 1), "workspace_admin.key_creator_denied")
	u.members[0].Role = "admin"
	assertCode(t, s.CheckKeyUse(context.Background(), "secret-101", 1), "workspace_admin.key_disabled")
	if !store.keys["1:101"].Disabled {
		t.Fatal("missing permanent revocation")
	}
	if err := s.CheckKeyUse(context.Background(), "unknown-legacy", 0); err != nil {
		t.Fatal(err)
	}
}
func TestCreatorGenerationClosesSuspensionCreationRace(t *testing.T) {
	s, store, u := fixture(t)
	u.onCreate = func() {
		if err := s.DisableCreatorKeys(context.Background(), "a"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 1, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if store.keys["1:101"].CreatorGeneration != 0 || store.generations["a"] != 1 {
		t.Fatal("snapshot ordering lost")
	}
	assertCode(t, s.CheckKeyUse(context.Background(), "secret-101", 1), "workspace_admin.key_disabled")
}
func TestTransientIdentityFailureDoesNotPermanentlyDisable(t *testing.T) {
	s, store, u := fixture(t)
	_, err := s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 1, validInput())
	if err != nil {
		t.Fatal(err)
	}
	s.IdentityAllowed = func(context.Context, string) error { return failure("identity.unavailable", 503) }
	assertCode(t, s.CheckKeyUse(context.Background(), "secret-101", 1), "workspace_admin.identity_unavailable")
	if store.keys["1:101"].Disabled {
		t.Fatal("temporary outage disabled key")
	}
	s.IdentityAllowed = func(_ context.Context, id string) error {
		if id == "a" {
			return failure("identity.denied", 403)
		}
		return nil
	}
	assertCode(t, s.CheckKeyUse(context.Background(), "secret-101", 1), "workspace_admin.key_creator_denied")
	if !store.keys["1:101"].Disabled {
		t.Fatal("suspension did not disable key")
	}
}
func TestDelegationUsesServerIssuedEncryptedSessionAndOwnerOnly(t *testing.T) {
	s, store, u := fixture(t)
	ctx := context.Background()
	_, err := s.SetDelegation(ctx, u.principals["admin"], header("admin"), 1, DelegationInput{})
	assertCode(t, err, "workspace_admin.owner_required")
	status, err := s.SetDelegation(ctx, u.principals["owner"], header("owner"), 1, DelegationInput{})
	if err != nil || !status.Authorized {
		t.Fatal(status, err)
	}
	cipher := store.delegations[1].SessionCipher
	if strings.Contains(string(cipher), "server-refresh") || strings.Contains(string(cipher), "delegated") {
		t.Fatal("plaintext session persisted")
	}
	if _, err := s.open(2, "session", cipher); err == nil {
		t.Fatal("tenant substitution decrypted")
	}
	if err := s.RevokeDelegation(ctx, u.principals["owner"], header("owner"), 1); err != nil {
		t.Fatal(err)
	}
	for _, call := range u.calls {
		if strings.Contains(call, "logout") {
			t.Fatal("revocation logged Owner out")
		}
	}
}
func TestDelegationRejectsSameSecondBrowserTokenAndRefreshesExpired(t *testing.T) {
	s, store, u := fixture(t)
	u.mintedToken = "owner"
	_, err := s.SetDelegation(context.Background(), u.principals["owner"], header("owner"), 1, DelegationInput{})
	assertCode(t, err, "workspace_admin.delegation_retry")
	u.principalErrors["delegated"] = &weknora.Error{UpstreamStatus: 401}
	u.refreshAccess = "renewed"
	u.principals["renewed"] = principal("o", "owner", 1)
	status, err := s.GetDelegationStatus(context.Background(), u.principals["admin"], header("admin"), 1)
	if err != nil || !status.Authorized {
		t.Fatal(status, err)
	}
	plain, _ := s.open(1, "session", store.delegations[1].SessionCipher)
	if !strings.Contains(string(plain), "rotated-refresh") {
		t.Fatal("rotation not persisted")
	}
	u.principals["renewed"] = principal("o", "viewer", 1)
	status, err = s.GetDelegationStatus(context.Background(), u.principals["admin"], header("admin"), 1)
	if err != nil || status.Authorized {
		t.Fatal("demoted Owner still authorized", status, err)
	}
}

func TestDelegationRefreshCannotSwitchToAnotherIdentity(t *testing.T) {
	s, _, u := fixture(t)
	u.principalErrors["delegated"] = &weknora.Error{UpstreamStatus: 401}
	u.refreshAccess = "wrong-owner"
	u.principals["wrong-owner"] = principal("another-owner", "owner", 1)
	status, err := s.GetDelegationStatus(context.Background(), u.principals["admin"], header("admin"), 1)
	if err != nil || status.Authorized || status.ErrorCode != "workspace_admin.delegation_owner_invalid" {
		t.Fatalf("refresh changed delegated identity: %#v %v", status, err)
	}
}

func TestDelegationRefreshRebindsOwnersLastActiveWorkspace(t *testing.T) {
	s, store, u := fixture(t)
	u.principalErrors["delegated"] = &weknora.Error{UpstreamStatus: 401}
	u.refreshAccess = "other-space"
	u.principals["other-space"] = weknora.Principal{
		User: &weknora.User{ID: "o", IsActive: true}, Tenant: &weknora.Tenant{ID: 2},
		Memberships: []weknora.Membership{{TenantID: 1, Role: "owner"}, {TenantID: 2, Role: "owner"}},
	}
	u.mintedToken = "rebound"
	u.principals["rebound"] = principal("o", "owner", 1)
	status, err := s.GetDelegationStatus(context.Background(), u.principals["admin"], header("admin"), 1)
	if err != nil || !status.Authorized {
		t.Fatal(status, err)
	}
	plain, err := s.open(1, "session", store.delegations[1].SessionCipher)
	if err != nil || !strings.Contains(string(plain), "rebound") {
		t.Fatal("workspace rebound session not persisted", err)
	}
}
func TestProtectedInternalKeysHaveNoSecretAndCannotMutate(t *testing.T) {
	s, _, u := fixture(t)
	u.keys[9] = keyDTO{ID: 9, ScopeType: "tenant", Name: internalPrefix + "members", APIKey: "internal-secret"}
	raw, err := s.ListKeys(context.Background(), u.principals["owner"], header("owner"), 1)
	if err != nil || strings.Contains(string(raw), "internal-secret") || !strings.Contains(string(raw), `"protected":true`) {
		t.Fatalf("bad protected view %s %v", raw, err)
	}
	err = s.RevokeKey(context.Background(), u.principals["owner"], header("owner"), 1, 9)
	assertCode(t, err, "workspace_admin.key_protected")
}

func TestDelegationRevokeDisablesProxyDuringUpstreamOutage(t *testing.T) {
	s, store, u := fixture(t)
	d := store.delegations[1]
	d.MemberKeyID = 9
	store.delegations[1] = d
	u.failDelete = true
	err := s.RevokeDelegation(context.Background(), u.principals["owner"], header("owner"), 1)
	assertCode(t, err, "workspace_admin.delegation_revoke_failed")
	if len(store.delegations[1].SessionCipher) != 0 || store.delegations[1].MemberKeyID != 9 {
		t.Fatal("revoked proxy remained active or lost cleanup reference")
	}
	_, err = s.delegatedHeaders(context.Background(), store, 1)
	assertCode(t, err, "workspace_admin.delegation_unavailable")
	u.failDelete = false
	if err := s.RevokeDelegation(context.Background(), u.principals["owner"], header("owner"), 1); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerCannotExpandOtherCreatorsPrivateScope(t *testing.T) {
	s, _, u := fixture(t)
	_, err := s.CreateKey(context.Background(), u.principals["admin"], header("admin"), 1, validInput())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateKey(context.Background(), u.principals["owner"], header("owner"), 1, 101, json.RawMessage(`{"name":"x","knowledge_base_ids":["owners-private-kb"],"capabilities":["retrieve"]}`))
	assertCode(t, err, "workspace_admin.creator_scope_expansion_denied")
	_, err = s.UpdateKey(context.Background(), u.principals["owner"], header("owner"), 1, 101, validInput())
	if err != nil {
		t.Fatal(err)
	}
}
