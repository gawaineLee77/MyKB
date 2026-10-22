package workspaceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type memberTestStore struct {
	*memoryStore
	mu sync.Mutex
}

func (m *memberTestStore) WithLock(_ context.Context, _ string, fn func(Store) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(m)
}

type memberWrite struct {
	method, path string
	header       http.Header
	input        map[string]any
}

type memberTestUpstream struct {
	*fakeUpstream
	writes          []memberWrite
	invitations     []map[string]any
	beforeWrite     func(memberWrite)
	writeError      error
	memberReadError error
	principalMode   string
	requireDirect   bool
}

func (u *memberTestUpstream) EnterpriseRequest(ctx context.Context, method, path string, h http.Header, input any) (json.RawMessage, error) {
	if method == http.MethodGet && strings.HasSuffix(path, "/api-principal-config") {
		mode := u.principalMode
		if mode == "" {
			mode = "tenant"
		}
		return encode(map[string]any{"success": true, "data": map[string]any{"mode": mode, "require_direct_header": u.requireDirect}}), nil
	}
	if method == http.MethodGet && strings.Contains(path, "/members?") && u.memberReadError != nil {
		return nil, u.memberReadError
	}
	if method == http.MethodGet && strings.Contains(path, "/invitations?") {
		return encode(map[string]any{"success": true, "data": map[string]any{"invitations": u.invitations, "total": len(u.invitations)}}), nil
	}
	if strings.Contains(path, "/api-keys") || strings.HasPrefix(path, "/api/v1/auth/") || method == http.MethodGet {
		return u.fakeUpstream.EnterpriseRequest(ctx, method, path, h, input)
	}
	var body map[string]any
	_ = json.Unmarshal(encode(input), &body)
	call := memberWrite{method, path, h.Clone(), body}
	u.writes = append(u.writes, call)
	if u.beforeWrite != nil {
		u.beforeWrite(call)
	}
	if u.writeError != nil {
		return nil, u.writeError
	}
	if strings.Contains(path, "/members/") && method == http.MethodPut {
		target := path[strings.LastIndex(path, "/")+1:]
		assigned := body["role"].(string)
		for i := range u.members {
			if u.members[i].UserID == target {
				u.members[i].Role = assigned
			}
		}
		for token, p := range u.principals {
			if p.User.ID == target {
				p.Memberships = []weknora.Membership{{TenantID: 1, Role: assigned}}
				u.principals[token] = p
			}
		}
	}
	return encode(map[string]bool{"success": true}), nil
}

func memberFixture(t *testing.T) (*Service, *memberTestStore, *memberTestUpstream) {
	t.Helper()
	s, store, upstream := fixture(t)
	locked := &memberTestStore{memoryStore: store}
	u := &memberTestUpstream{fakeUpstream: upstream}
	s.Store, s.Upstream = locked, u
	u.members = []weknora.TenantMember{
		{UserID: "o", Email: "owner@example.test", Role: "owner", Status: "active"},
		{UserID: "a", Email: "admin@example.test", Role: "admin", Status: "active"},
		{UserID: "b", Email: "other@example.test", Role: "admin", Status: "active"},
		{UserID: "v", Email: "viewer@example.test", Role: "viewer", Status: "active"},
	}
	return s, locked, u
}

func TestAdminMemberAndWorkspaceOperations(t *testing.T) {
	tests := []struct{ method, suffix, body string }{
		{http.MethodPost, "/members", `{"email":"new@example.test","role":"contributor"}`},
		{http.MethodPost, "/invitations", `{"email":"new@example.test","role":"admin","message":"Join"}`},
		{http.MethodPost, "/invite-links", `{"role":"viewer"}`},
		{http.MethodPut, "/members/v", `{"role":"contributor"}`},
		{http.MethodDelete, "/members/b", ``},
		{http.MethodDelete, "/invitations/7", ``},
		{http.MethodPut, "", `{"name":"Renamed","description":"Workspace"}`},
	}
	for _, tc := range tests {
		t.Run(tc.method+tc.suffix, func(t *testing.T) {
			s, store, u := memberFixture(t)
			u.invitations = []map[string]any{{"id": 7, "tenant_id": 1, "role": "viewer", "invitee_user_id": "v"}}
			_, err := s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, tc.method, tc.suffix, json.RawMessage(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if len(u.writes) != 1 {
				t.Fatalf("writes = %#v", u.writes)
			}
			forward := u.writes[0].header
			if tc.suffix == "" {
				if forward.Get("Authorization") != "Bearer delegated" || forward.Get("X-API-Key") != "" {
					t.Fatalf("workspace update missing delegated session: %#v", forward)
				}
			} else {
				if forward.Get("Authorization") != "" || forward.Get("X-API-Key") == "" || forward.Get("X-Tenant-ID") != "1" {
					t.Fatalf("member write did not use private machine credential: %#v", forward)
				}
				d := store.delegations[1]
				key := u.keys[d.MemberKeyID]
				if key.FullAccess || len(key.Capabilities) != 1 || key.Capabilities[0] != "manage_members" || !strings.HasPrefix(key.Name, "mindcreek-internal:") || len(d.MemberKeyCipher) == 0 {
					t.Fatalf("incorrect private credential: %#v", key)
				}
			}
			if len(store.audits) < 2 || store.audits[0].ActorUserID != "a" || store.audits[len(store.audits)-1].Outcome != "success" {
				t.Fatalf("missing human attribution: %#v", store.audits)
			}
		})
	}
}

func TestAdminCannotManageOwnerOrGrantOwnership(t *testing.T) {
	tests := []struct{ method, suffix, body string }{
		{http.MethodPost, "/members", `{"email":"new@example.test","role":"owner"}`},
		{http.MethodPost, "/invitations", `{"email":"new@example.test","role":"owner"}`},
		{http.MethodPost, "/invite-links", `{"role":"owner"}`},
		{http.MethodPost, "/members", `{"email":"owner@example.test","role":"viewer"}`},
		{http.MethodPost, "/invitations", `{"email":"owner@example.test","role":"viewer"}`},
		{http.MethodPut, "/members/v", `{"role":"owner"}`},
		{http.MethodPut, "/members/o", `{"role":"viewer"}`},
		{http.MethodDelete, "/members/o", ``},
		{http.MethodDelete, "/invitations/7", ``},
	}
	for _, tc := range tests {
		t.Run(tc.method+tc.suffix+tc.body, func(t *testing.T) {
			s, _, u := memberFixture(t)
			u.invitations = []map[string]any{{"id": 7, "tenant_id": 1, "role": "owner"}}
			_, err := s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, tc.method, tc.suffix, json.RawMessage(tc.body))
			assertCode(t, err, "workspace.owner_protected")
			if len(u.writes) != 0 || len(u.keys) != 0 {
				t.Fatal("denied operation reached privileged write")
			}
		})
	}
}

func TestMemberInputAndTenantBoundaries(t *testing.T) {
	for _, body := range []string{`{"name":"x","storage_quota":999}`, `{"name":"x","tenant_id":2}`, `{"name":" "}`, `null`, `{}`, `{"name":"x"} {}`} {
		s, _, u := memberFixture(t)
		_, err := s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodPut, "", json.RawMessage(body))
		assertCode(t, err, "workspace.request_invalid")
		if len(u.writes) != 0 {
			t.Fatal("invalid request mutated workspace")
		}
	}
	s, _, u := memberFixture(t)
	for _, actor := range []string{"viewer", "admin"} {
		tenant, code := uint64(1), "workspace_admin.admin_required"
		if actor == "admin" {
			tenant, code = 2, "workspace_admin.access_denied"
		}
		_, err := s.Manage(context.Background(), u.principals[actor], header(actor), tenant, http.MethodPut, "", json.RawMessage(`{"name":"x"}`))
		assertCode(t, err, code)
	}
	h := header("admin")
	h.Set("X-API-Key", "machine")
	_, err := s.Manage(context.Background(), u.principals["admin"], h, 1, http.MethodPut, "", json.RawMessage(`{"name":"x"}`))
	assertCode(t, err, "workspace_admin.human_required")
	u.invitations = []map[string]any{{"id": 7, "tenant_id": 2, "role": "viewer"}}
	_, err = s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodDelete, "/invitations/7", nil)
	assertCode(t, err, "workspace.invitation_not_found")
}

func TestMembershipLossDisablesKeysBeforeNativeWrite(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			s, store, u := memberFixture(t)
			store.keys["1:9"] = KeyOwner{TenantID: 1, KeyID: 9, CreatorUserID: "b"}
			u.beforeWrite = func(memberWrite) {
				if !store.keys["1:9"].Disabled {
					t.Fatal("native membership changed before key revocation")
				}
			}
			var body json.RawMessage
			if method == http.MethodPut {
				body = json.RawMessage(`{"role":"viewer"}`)
			}
			u.writeError = errors.New("native mutation unavailable")
			_, err := s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, method, "/members/b", body)
			assertCode(t, err, "workspace.member_update_failed_keys_disabled")
			if !store.keys["1:9"].Disabled {
				t.Fatal("ambiguous membership write revived key")
			}
		})
	}
}

func TestOwnerMutationAndLastActiveOwnerProtection(t *testing.T) {
	s, store, u := memberFixture(t)
	_, err := s.Manage(context.Background(), u.principals["owner"], header("owner"), 1, http.MethodPut, "/members/b", json.RawMessage(`{"role":"owner"}`))
	if err != nil || u.writes[0].header.Get("Authorization") != "Bearer owner" {
		t.Fatalf("Owner promotion %v", err)
	}
	store.keys["1:9"] = KeyOwner{TenantID: 1, KeyID: 9, CreatorUserID: "o"}
	for i := range u.members {
		if u.members[i].UserID == "b" {
			u.members[i].Status = "inactive"
		}
	}
	_, err = s.Manage(context.Background(), u.principals["owner"], header("owner"), 1, http.MethodPut, "/members/o", json.RawMessage(`{"role":"admin"}`))
	assertCode(t, err, "workspace.last_owner")
	if store.keys["1:9"].Disabled || len(u.writes) != 1 {
		t.Fatal("last Owner denial changed memberships or keys")
	}
}

func TestOwnerPromotionSerializesAdminTargetCheck(t *testing.T) {
	s, _, u := memberFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	u.beforeWrite = func(call memberWrite) {
		if call.method == http.MethodPut {
			close(entered)
			<-release
		}
	}
	owner, admin := u.principals["owner"], u.principals["admin"]
	ownerDone, adminDone := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := s.Manage(context.Background(), owner, header("owner"), 1, http.MethodPut, "/members/b", json.RawMessage(`{"role":"owner"}`))
		ownerDone <- err
	}()
	<-entered
	go func() {
		_, err := s.Manage(context.Background(), admin, header("admin"), 1, http.MethodDelete, "/members/b", nil)
		adminDone <- err
	}()
	select {
	case err := <-adminDone:
		close(release)
		t.Fatalf("Admin escaped workspace lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	assertCode(t, <-adminDone, "workspace.owner_protected")
	if len(u.writes) != 1 {
		t.Fatalf("concurrent Admin removed promoted Owner: %#v", u.writes)
	}
}

func TestSelfLeaveForEveryRoleAndKeyLifecycle(t *testing.T) {
	for _, actor := range []string{"admin", "viewer"} {
		t.Run(actor, func(t *testing.T) {
			s, store, u := memberFixture(t)
			p := u.principals[actor]
			store.keys["1:9"] = KeyOwner{TenantID: 1, KeyID: 9, CreatorUserID: p.User.ID}
			_, err := s.SelfLeave(context.Background(), p, header(actor), 1)
			if err != nil || !store.keys["1:9"].Disabled || len(u.writes) != 1 || u.writes[0].header.Get("Authorization") != "Bearer "+actor {
				t.Fatalf("leave %s: %v %#v", actor, err, u.writes)
			}
		})
	}
	s, _, u := memberFixture(t)
	_, err := s.SelfLeave(context.Background(), u.principals["owner"], header("owner"), 1)
	assertCode(t, err, "workspace.last_owner")
	s.IdentityAllowed = func(context.Context, string) error { return fmt.Errorf("database unavailable") }
	_, err = s.SelfLeave(context.Background(), u.principals["viewer"], header("viewer"), 1)
	assertCode(t, err, "workspace_admin.identity_unavailable")
}

func TestInternalMemberCredentialRecoversAmbiguousCreate(t *testing.T) {
	s, store, u := memberFixture(t)
	u.lostCreate = true
	input := json.RawMessage(`{"email":"new@example.test","role":"viewer"}`)
	_, err := s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodPost, "/members", input)
	if err == nil || len(u.writes) != 0 {
		t.Fatal("ambiguous credential creation must stop mutation")
	}
	u.lostCreate = false
	_, err = s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodPost, "/members", input)
	if err != nil || len(u.keys) != 1 || store.delegations[1].MemberKeyID == 0 || len(u.writes) != 1 {
		t.Fatalf("recovery failed: %v keys=%d", err, len(u.keys))
	}
}

func TestMachineMemberMutationPreservesCredentialAndRevokesCreatorKeys(t *testing.T) {
	s, store, u := memberFixture(t)
	store.keys["1:9"] = KeyOwner{TenantID: 1, KeyID: 9, CreatorUserID: "b", Fingerprint: fingerprint("creator-key")}
	u.beforeWrite = func(call memberWrite) {
		if !store.keys["1:9"].Disabled {
			t.Fatal("machine role mutation bypassed creator-key revocation")
		}
		if call.header.Get("X-API-Key") != "legacy-machine" || call.header.Get("Authorization") != "" {
			t.Fatalf("machine key exchanged for human/delegation credential: %#v", call.header)
		}
	}
	_, err := s.ManageMachine(context.Background(), memberKeyHeader("legacy-machine", 1), 1, http.MethodPut, "/members/b", json.RawMessage(`{"role":"viewer"}`))
	if err != nil || len(u.writes) != 1 || len(u.keys) != 0 {
		t.Fatalf("machine mutation failed: %v writes=%#v", err, u.writes)
	}
	if !strings.HasPrefix(store.audits[0].ActorUserID, "api-key:") || strings.Contains(store.audits[0].ActorUserID, "legacy-machine") {
		t.Fatalf("machine audit exposed secret or impersonated human: %#v", store.audits)
	}
}

func TestMachineNativeAuthorizationPrecedesKeyRevocation(t *testing.T) {
	s, store, u := memberFixture(t)
	store.keys["1:9"] = KeyOwner{TenantID: 1, KeyID: 9, CreatorUserID: "b"}
	u.memberReadError = &weknora.Error{Code: "upstream.forbidden", UpstreamStatus: 403, StatusCode: 403}
	_, err := s.ManageMachine(context.Background(), memberKeyHeader("untrusted-machine", 1), 1, http.MethodDelete, "/members/b", nil)
	if err == nil || len(u.writes) != 0 || store.keys["1:9"].Disabled || len(store.audits) != 0 {
		t.Fatalf("unauthorized machine changed local/upstream state: %v", err)
	}
	u.memberReadError = nil
	_, err = s.ManageMachine(context.Background(), memberKeyHeader("legacy-machine", 2), 1, http.MethodDelete, "/members/b", nil)
	assertCode(t, err, "workspace_admin.key_tenant_denied")
	_, err = s.ManageMachine(context.Background(), memberKeyHeader("legacy-machine", 1), 1, http.MethodPut, "/members/b", json.RawMessage(`{"role":"owner"}`))
	assertCode(t, err, "workspace.machine_owner_grant_denied")
	store.keys["1:10"] = KeyOwner{TenantID: 1, KeyID: 10, CreatorUserID: "o", Fingerprint: fingerprint("disabled-machine"), Disabled: true}
	_, err = s.ManageMachine(context.Background(), memberKeyHeader("disabled-machine", 1), 1, http.MethodDelete, "/members/b", nil)
	assertCode(t, err, "workspace_admin.key_disabled")
	if len(u.writes) != 0 || store.keys["1:9"].Disabled {
		t.Fatal("denied machine bypassed lifecycle check")
	}
}

func TestMachineRoleChangeSerializesHumanReauthorization(t *testing.T) {
	s, _, u := memberFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	u.beforeWrite = func(call memberWrite) {
		if call.method == http.MethodPut {
			close(entered)
			<-release
		}
	}
	admin := u.principals["admin"]
	machineDone, adminDone := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := s.ManageMachine(context.Background(), memberKeyHeader("legacy-machine", 1), 1, http.MethodPut, "/members/a", json.RawMessage(`{"role":"viewer"}`))
		machineDone <- err
	}()
	<-entered
	go func() {
		_, err := s.Manage(context.Background(), admin, header("admin"), 1, http.MethodDelete, "/members/b", nil)
		adminDone <- err
	}()
	select {
	case err := <-adminDone:
		close(release)
		t.Fatalf("human escaped machine workspace lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-machineDone; err != nil {
		t.Fatal(err)
	}
	assertCode(t, <-adminDone, "workspace_admin.admin_required")
	if len(u.writes) != 1 {
		t.Fatal("stale Admin authorization survived machine demotion")
	}
}

func TestAdminMemberOperationsRespectNativePrincipalMode(t *testing.T) {
	for _, tc := range []struct {
		mode                     string
		requireDirect, wantOwner bool
	}{{"tenant", false, false}, {"direct_header", false, false}, {"direct_header", true, true}, {"signed_token", false, true}} {
		t.Run(fmt.Sprintf("%s-required=%t", tc.mode, tc.requireDirect), func(t *testing.T) {
			s, _, u := memberFixture(t)
			u.principalMode, u.requireDirect = tc.mode, tc.requireDirect
			_, err := s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodPost, "/members", json.RawMessage(`{"email":"new@example.test","role":"viewer"}`))
			if err != nil || len(u.writes) != 1 {
				t.Fatalf("native principal mode blocked member operation: %v", err)
			}
			forward := u.writes[0].header
			if tc.wantOwner {
				if forward.Get("Authorization") != "Bearer delegated" || forward.Get("X-API-Key") != "" || len(u.keys) != 0 {
					t.Fatalf("proof-required mode should use constrained human proxy: %#v", forward)
				}
			} else if forward.Get("Authorization") != "" || forward.Get("X-API-Key") == "" {
				t.Fatalf("tenant principal mode should keep restricted machine credential: %#v", forward)
			}
			_, err = s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodPut, "/members/o", json.RawMessage(`{"role":"viewer"}`))
			assertCode(t, err, "workspace.owner_protected")
			_, err = s.Manage(context.Background(), u.principals["admin"], header("admin"), 1, http.MethodPost, "/invite-links", json.RawMessage(`{"role":"owner"}`))
			assertCode(t, err, "workspace.owner_protected")
		})
	}
}

func TestMachineMemberProofHeadersArePreserved(t *testing.T) {
	s, _, u := memberFixture(t)
	h := memberKeyHeader("legacy-machine", 1)
	h.Set("X-External-User-ID", "client-user")
	h.Set("X-External-User-Token", "synthetic-signed-proof")
	_, err := s.ManageMachine(context.Background(), h, 1, http.MethodPost, "/members", json.RawMessage(`{"email":"new@example.test","role":"viewer"}`))
	if err != nil {
		t.Fatal(err)
	}
	forward := u.writes[0].header
	if forward.Get("X-External-User-ID") != "client-user" || forward.Get("X-External-User-Token") != "synthetic-signed-proof" || forward.Get("Authorization") != "" {
		t.Fatalf("native external principal proof lost: %#v", forward)
	}
}
