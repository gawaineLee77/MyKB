package kbacl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type memoryStore struct {
	policies  map[string]Policy
	gens      map[string]Generation
	creations map[string]CreateOperation
	events    []Event
	fail      error
}

func newMemory() *memoryStore {
	return &memoryStore{policies: map[string]Policy{}, gens: map[string]Generation{}, creations: map[string]CreateOperation{}}
}
func recordKey(tenant uint64, id string) string { return fmt.Sprintf("%d:%s", tenant, id) }
func (m *memoryStore) Policy(_ context.Context, t uint64, k string) (Policy, error) {
	if m.fail != nil {
		return Policy{}, m.fail
	}
	p, ok := m.policies[recordKey(t, k)]
	if !ok {
		return p, ErrNotFound
	}
	return p, nil
}
func (m *memoryStore) Generation(_ context.Context, t uint64, u string) (Generation, error) {
	return m.gens[recordKey(t, u)], m.fail
}
func (m *memoryStore) Ensure(_ context.Context, p Policy) (Policy, error) {
	if existing, ok := m.policies[recordKey(p.TenantID, p.KBID)]; ok {
		return existing, nil
	}
	m.policies[recordKey(p.TenantID, p.KBID)] = p
	return p, m.fail
}
func (m *memoryStore) Save(_ context.Context, p Policy, revision uint64, e Event) (Policy, error) {
	if m.fail != nil {
		return Policy{}, m.fail
	}
	old, ok := m.policies[recordKey(p.TenantID, p.KBID)]
	if !ok || old.Revision != revision || old.Deleted {
		return Policy{}, ErrConflict
	}
	p.Revision = revision + 1
	m.policies[recordKey(p.TenantID, p.KBID)] = p
	m.events = append(m.events, e)
	return p, nil
}
func (m *memoryStore) Events(context.Context, uint64, string) ([]Event, error) {
	return m.events, m.fail
}
func (m *memoryStore) RevokeMember(_ context.Context, tenant uint64, user string, remove bool) error {
	if remove {
		g := m.gens[recordKey(tenant, user)]
		g.Member++
		m.gens[recordKey(tenant, user)] = g
	}
	for key, p := range m.policies {
		if p.TenantID != tenant {
			continue
		}
		if p.CreatorUserID == user && remove {
			p.CreatorRevoked = true
		}
		members := []Member{}
		for _, v := range p.Members {
			if v.UserID == user {
				if remove {
					continue
				}
				v.Operations = []string{}
			}
			members = append(members, v)
		}
		p.Members = members
		p.Revision++
		m.policies[key] = p
	}
	return m.fail
}
func (m *memoryStore) RevokeIdentity(ctx context.Context, user string) error {
	for _, p := range m.policies {
		g := m.gens[recordKey(p.TenantID, user)]
		g.Identity++
		m.gens[recordKey(p.TenantID, user)] = g
	}
	return m.fail
}
func (m *memoryStore) BeginCreation(_ context.Context, op CreateOperation) (CreateOperation, bool, error) {
	for _, old := range m.creations {
		if old.TenantID == op.TenantID && old.ActorUserID == op.ActorUserID && old.KeyHash == op.KeyHash {
			return old, false, nil
		}
	}
	m.creations[op.ID] = op
	return op, true, m.fail
}
func (m *memoryStore) Creation(_ context.Context, t uint64, id string) (CreateOperation, error) {
	op, ok := m.creations[id]
	if !ok || op.TenantID != t {
		return op, ErrNotFound
	}
	return op, m.fail
}
func (m *memoryStore) CreationByKey(_ context.Context, t uint64, u, k string) (CreateOperation, error) {
	for _, op := range m.creations {
		if op.TenantID == t && op.ActorUserID == u && op.KeyHash == k {
			return op, nil
		}
	}
	return CreateOperation{}, ErrNotFound
}
func (m *memoryStore) CompleteCreation(_ context.Context, t uint64, id, kb string) error {
	op, ok := m.creations[id]
	if !ok || op.TenantID != t || op.State != "pending" {
		return ErrConflict
	}
	op.State = "completed"
	op.KBID = kb
	m.creations[id] = op
	return m.fail
}
func (m *memoryStore) Creations(context.Context, uint64) ([]CreateOperation, error) {
	result := []CreateOperation{}
	for _, op := range m.creations {
		result = append(result, op)
	}
	return result, m.fail
}
func (m *memoryStore) CreationPending(_ context.Context, tenant uint64, creator, kb string) (bool, error) {
	pending := false
	for _, op := range m.creations {
		if op.TenantID != tenant {
			continue
		}
		if op.State == "completed" && op.KBID == kb {
			return false, m.fail
		}
		if op.State == "pending" && op.ActorUserID == creator && !contains(op.BaselineIDs, kb) {
			pending = true
		}
	}
	return pending, m.fail
}

type nativeStub struct {
	p         weknora.Principal
	kb        kbIdentity
	err       error
	calls     int
	list      []kbIdentity
	inventory json.RawMessage
}

func (n *nativeStub) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return n.p, n.err
}
func (n *nativeStub) NativeRequest(_ context.Context, _ string, path string, _ url.Values, _ http.Header, _ any) (json.RawMessage, error) {
	n.calls++
	if n.err != nil {
		return nil, n.err
	}
	if path == "/api/v1/knowledge-bases" {
		if n.inventory != nil {
			return n.inventory, nil
		}
		rows := n.list
		if rows == nil {
			rows = []kbIdentity{}
		}
		raw, _ := json.Marshal(map[string]any{"success": true, "data": rows})
		return raw, nil
	}
	kb := n.kb
	if kb.ID == "" {
		kb = kbIdentity{ID: strings.TrimPrefix(path, "/api/v1/knowledge-bases/"), TenantID: 7, CreatorID: "creator"}
	}
	raw, _ := json.Marshal(map[string]any{"success": true, "data": kb})
	return raw, nil
}
func setup(user, role string) (*Service, *memoryStore, *nativeStub, http.Header) {
	m := newMemory()
	n := &nativeStub{p: weknora.Principal{User: &weknora.User{ID: user, IsActive: true}, Tenant: &weknora.Tenant{ID: 7}, Memberships: []weknora.Membership{{TenantID: 7, Role: role}}}}
	s := &Service{Store: m, Upstream: n, IdentityAllowed: func(context.Context, string) error { return nil }, WithAuthorizationLock: func(_ context.Context, _ http.Header, _ []string, fn func() error) error { return fn() }, Members: func(context.Context, http.Header, uint64) ([]weknora.TenantMember, error) {
		return []weknora.TenantMember{{UserID: "creator", Username: "Creator", Role: "contributor", Status: "active"}, {UserID: "collaborator", Username: "Collaborator", Role: "contributor", Status: "active"}, {UserID: "viewer", Username: "Viewer", Role: "viewer", Status: "active"}}, nil
	}}
	return s, m, n, http.Header{"Authorization": {"Bearer human"}}
}
func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil || statusOf(err) != want {
		t.Fatalf("error = %v, want status %d", err, want)
	}
}
func TestDefaultPrivateMatrixAndRoleCeilings(t *testing.T) {
	for _, tc := range []struct {
		user, role               string
		visible, manage, creator bool
	}{{"owner", "owner", true, true, false}, {"admin", "admin", true, true, false}, {"creator", "contributor", true, true, true}, {"creator", "viewer", true, false, true}, {"other", "contributor", false, false, false}, {"viewer", "viewer", false, false, false}} {
		t.Run(tc.user+tc.role, func(t *testing.T) {
			s, _, _, h := setup(tc.user, tc.role)
			p, err := s.Authorize(context.Background(), h, "kb")
			if !tc.visible {
				assertStatus(t, err, 404)
				return
			}
			if err != nil || p.Visible != tc.visible || p.Manage != tc.manage || p.Creator != tc.creator {
				t.Fatalf("permissions=%+v error=%v", p, err)
			}
			if tc.manage && len(p.Operations) != 7 {
				t.Fatal("manager missing operations")
			}
			if !tc.manage && len(p.Operations) != 0 {
				t.Fatal("viewer creator inherited writes")
			}
		})
	}
}
func TestVisibleContributorReceivesOnlyExplicitOperations(t *testing.T) {
	s, m, _, h := setup("collaborator", "contributor")
	m.policies["7:kb"] = Policy{TenantID: 7, KBID: "kb", CreatorUserID: "creator", Visibility: "restricted", Revision: 3, Members: []Member{{UserID: "collaborator", Visible: true, Operations: []string{"document.create"}}}}
	if _, err := s.Authorize(context.Background(), h, "kb", "document.create"); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"manage", "document.delete", "document.download", "chunk.update", "unknown"} {
		_, err := s.Authorize(context.Background(), h, "kb", op)
		assertStatus(t, err, 403)
	}
	s.Upstream.(*nativeStub).p.Memberships[0].Role = "viewer"
	_, err := s.Authorize(context.Background(), h, "kb", "document.create")
	assertStatus(t, err, 403)
}
func TestWorkspaceVisibilityDoesNotGrantWriteOrCrossTenantAccess(t *testing.T) {
	s, m, n, h := setup("viewer", "viewer")
	m.policies["7:kb"] = Policy{TenantID: 7, KBID: "kb", Visibility: "workspace", Revision: 1}
	if _, err := s.Authorize(context.Background(), h, "kb"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Authorize(context.Background(), h, "kb", "document.download")
	assertStatus(t, err, 403)
	n.kb = kbIdentity{ID: "kb", TenantID: 8, CreatorID: "viewer"}
	_, err = s.Authorize(context.Background(), h, "kb")
	assertStatus(t, err, 404)
}

func TestTemporaryHistoryKBIsNotAnACLManagedContentObject(t *testing.T) {
	s, m, n, h := setup("owner", "owner")
	n.kb = kbIdentity{ID: "internal-history", TenantID: 7, CreatorID: "owner", IsTemporary: true}
	_, err := s.Authorize(context.Background(), h, "internal-history")
	assertStatus(t, err, 404)
	if len(m.policies) != 0 {
		t.Fatal("temporary KB became an ordinary ACL object")
	}
	_, err = s.UpdatePolicy(context.Background(), h, "internal-history", PolicyInput{Revision: 1, Visibility: "workspace", Members: []Member{}})
	assertStatus(t, err, 404)
}
func TestMachineDoesNotInheritProjectedOwnerOrManagePolicy(t *testing.T) {
	s, _, _, h := setup("projected-owner", "owner")
	h.Set("X-API-Key", "machine")
	_, err := s.Authorize(context.Background(), h, "kb")
	assertStatus(t, err, 404)
	s.MachinePermissions = func(context.Context, http.Header, string) (Permissions, error) {
		return Permissions{Visible: true, Manage: true, Creator: true, Operations: []string{"document.create"}}, nil
	}
	p, err := s.Authorize(context.Background(), h, "kb", "document.create")
	if err != nil || p.Manage || p.Creator {
		t.Fatalf("machine %+v %v", p, err)
	}
	_, err = s.Authorize(context.Background(), h, "kb", "manage")
	assertStatus(t, err, 403)
	_, err = s.UpdatePolicy(context.Background(), h, "kb", PolicyInput{Revision: 1, Visibility: "workspace", Members: []Member{}})
	assertStatus(t, err, 403)
}
func TestPolicySnapshotRevisionValidationAndNonDelegation(t *testing.T) {
	s, m, _, h := setup("creator", "contributor")
	if _, err := s.Authorize(context.Background(), h, "kb"); err != nil {
		t.Fatal(err)
	}
	input := PolicyInput{Revision: 1, Visibility: "restricted", Members: []Member{{UserID: "collaborator", Visible: true, Operations: []string{"chunk.update"}}}}
	p, err := s.UpdatePolicy(context.Background(), h, "kb", input)
	if err != nil || p.Revision != 2 || len(m.events) != 1 {
		t.Fatalf("save %+v %v", p, err)
	}
	_, err = s.UpdatePolicy(context.Background(), h, "kb", input)
	assertStatus(t, err, 409)
	input.Revision = 2
	input.Members[0].UserID = "viewer"
	_, err = s.UpdatePolicy(context.Background(), h, "kb", input)
	assertStatus(t, err, 400)
	input.Members = nil
	_, err = s.UpdatePolicy(context.Background(), h, "kb", input)
	assertStatus(t, err, 400)
	s.Upstream.(*nativeStub).p.User.ID = "collaborator"
	_, err = s.UpdatePolicy(context.Background(), h, "kb", PolicyInput{Revision: 2, Visibility: "workspace", Members: []Member{}})
	assertStatus(t, err, 403)
}
func TestLateGrantCannotSurviveIdentityGenerationChange(t *testing.T) {
	s, m, n, h := setup("creator", "contributor")
	if _, err := s.Authorize(context.Background(), h, "kb"); err != nil {
		t.Fatal(err)
	}
	s.IdentityAllowed = func(_ context.Context, user string) error {
		if user == "collaborator" {
			g := m.gens["7:collaborator"]
			g.Identity++
			m.gens["7:collaborator"] = g
		}
		return nil
	}
	_, err := s.UpdatePolicy(context.Background(), h, "kb", PolicyInput{Revision: 1, Visibility: "restricted", Members: []Member{{UserID: "collaborator", Visible: true, Operations: []string{"document.create"}}}})
	if err != nil {
		t.Fatal(err)
	}
	s.IdentityAllowed = func(context.Context, string) error { return nil }
	n.p.User.ID = "collaborator"
	_, err = s.Authorize(context.Background(), h, "kb")
	assertStatus(t, err, 404)
}
func TestCreatorRemovalLazyMigrationAndExplicitRestoration(t *testing.T) {
	s, m, n, h := setup("creator", "contributor")
	if err := s.RevokeMember(context.Background(), 7, "creator", true); err != nil {
		t.Fatal(err)
	}
	_, err := s.Authorize(context.Background(), h, "kb")
	assertStatus(t, err, 404)
	if !m.policies["7:kb"].CreatorRevoked {
		t.Fatal("lazy migration restored removed creator")
	}
	n.p.User.ID = "owner"
	n.p.Memberships[0].Role = "owner"
	p, err := s.RestoreCreator(context.Background(), h, "kb", 1)
	if err != nil || p.CreatorRevoked {
		t.Fatalf("restore %+v %v", p, err)
	}
	n.p.User.ID = "creator"
	n.p.Memberships[0].Role = "contributor"
	perms, err := s.Authorize(context.Background(), h, "kb", "manage")
	if err != nil || !perms.Creator {
		t.Fatal(err)
	}
}
func TestNewVerifiedCreationCanBindAfterPriorMembershipRevocation(t *testing.T) {
	s, m, _, h := setup("creator", "contributor")
	m.gens["7:creator"] = Generation{Member: 2, Identity: 1}
	if err := s.BindCreated(context.Background(), h, "new-kb"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authorize(context.Background(), h, "new-kb", "manage"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDeleted(context.Background(), h, "new-kb"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Authorize(context.Background(), h, "new-kb")
	assertStatus(t, err, 404)
	err = s.BindCreated(context.Background(), h, "new-kb")
	assertStatus(t, err, 404)
	_, err = s.Authorize(context.Background(), h, "new-kb")
	assertStatus(t, err, 404)
}
func TestAllowedKBFilterOnlyHidesInvisibleAndFailsClosedOnOutage(t *testing.T) {
	s, m, _, h := setup("viewer", "viewer")
	m.policies["7:public"] = Policy{TenantID: 7, KBID: "public", Visibility: "workspace", Revision: 1}
	ids, err := s.AllowedKnowledgeBases(context.Background(), h, []string{"private", "public", "public"})
	if err != nil || len(ids) != 1 || ids[0] != "public" {
		t.Fatalf("%v %v", ids, err)
	}
	m.fail = errors.New("postgres credentials must not leak")
	_, err = s.AllowedKnowledgeBases(context.Background(), h, []string{"public"})
	if err == nil {
		t.Fatal("outage silently filtered")
	}
	r := httptest.NewRequest(http.MethodGet, routePrefix+"public/effective", nil)
	r.Header = h
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "postgres") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestMachineScope404IsFilteredWithoutHidingOtherErrors(t *testing.T) {
	s, _, _, h := setup("projected-owner", "owner")
	h.Set("X-API-Key", "machine")
	s.MachinePermissions = func(_ context.Context, _ http.Header, kb string) (Permissions, error) {
		if kb == "hidden" {
			return Permissions{}, &weknora.Error{StatusCode: 404}
		}
		return Permissions{Visible: true}, nil
	}
	ids, err := s.AllowedKnowledgeBases(context.Background(), h, []string{"hidden", "visible"})
	if err != nil || len(ids) != 1 || ids[0] != "visible" {
		t.Fatal(ids, err)
	}
	s.MachinePermissions = func(context.Context, http.Header, string) (Permissions, error) {
		return Permissions{}, &weknora.Error{StatusCode: 503}
	}
	_, err = s.AllowedKnowledgeBases(context.Background(), h, []string{"visible"})
	assertStatus(t, err, 503)
}
func TestHTTPRejectsInjectedCreatorAndHidesRosterFromCollaborator(t *testing.T) {
	s, _, _, h := setup("creator", "contributor")
	r := httptest.NewRequest(http.MethodPut, routePrefix+"kb", strings.NewReader(`{"revision":1,"visibility":"workspace","members":[],"creator_user_id":"viewer"}`))
	r.Header = h
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest(http.MethodGet, routePrefix+"kb/members?q=Viewer", nil)
	r.Header = h
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "email") || !strings.Contains(w.Body.String(), "display_name") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	s.Upstream.(*nativeStub).p.User.ID = "viewer"
	s.Upstream.(*nativeStub).p.Memberships[0].Role = "viewer"
	r = httptest.NewRequest(http.MethodGet, routePrefix+"kb/events", nil)
	r.Header = h
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
func TestCreationSagaPreventsUnknownRetryAndCreatorForgery(t *testing.T) {
	s, m, n, h := setup("creator", "contributor")
	ctx := context.Background()
	hash := hashKey("body")
	op, err := s.BeginCreate(ctx, h, "request-1", hash)
	if err != nil || op.State != "pending" {
		t.Fatal(err)
	}
	_, err = s.BeginCreate(ctx, h, "request-1", hash)
	assertStatus(t, err, 409)
	_, err = s.BeginCreate(ctx, h, "request-1", hashKey("changed"))
	assertStatus(t, err, 409)
	n.kb = kbIdentity{ID: "kb", TenantID: 7, CreatorID: "other"}
	err = s.CompleteCreate(ctx, h, "request-1", "kb")
	assertStatus(t, err, 409)
	if m.creations[op.ID].State != "pending" {
		t.Fatal("uncertain create marked usable")
	}
	n.kb.CreatorID = "creator"
	if err = s.CompleteCreate(ctx, h, "request-1", "kb"); err != nil {
		t.Fatal(err)
	}
	if m.creations[op.ID].State != "completed" {
		t.Fatal("creation incomplete")
	}
	_, err = s.BeginCreate(ctx, h, "request-1", hash)
	assertStatus(t, err, 409)
}

func TestUncertainCreationDoesNotBecomeHistoricalOrPublicThroughInspection(t *testing.T) {
	s, _, n, h := setup("creator", "contributor")
	ctx := context.Background()
	n.list = []kbIdentity{{ID: "historical", TenantID: 7, CreatorID: "creator"}}
	op, err := s.BeginCreate(ctx, h, "request-1", hashKey("body"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, h, "historical"); err != nil {
		t.Fatal("pending request hid a historical KB", err)
	}
	_, err = s.Authorize(ctx, h, "unknown-result")
	assertStatus(t, err, 404)
	// Even a saved ACL from a partially completed saga cannot release the KB.
	if _, err = s.EnsurePolicy(ctx, h, "unknown-result"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Authorize(ctx, h, "unknown-result")
	assertStatus(t, err, 404)
	n.p.User.ID, n.p.Memberships[0].Role = "admin", "admin"
	policy, err := s.UpdatePolicy(ctx, h, "unknown-result", PolicyInput{Revision: 1, Visibility: "workspace", Members: []Member{}})
	if err != nil || policy.Visibility != "workspace" {
		t.Fatal("manager could not inspect/recover pending KB", err)
	}
	n.p.User.ID, n.p.Memberships[0].Role = "viewer", "viewer"
	_, err = s.Authorize(ctx, h, "unknown-result")
	assertStatus(t, err, 404)
	n.p.User.ID, n.p.Memberships[0].Role = "owner", "owner"
	h.Set("X-API-Key", "machine")
	s.MachinePermissions = func(context.Context, http.Header, string) (Permissions, error) {
		t.Fatal("machine reached independent grants for a pending KB")
		return Permissions{Visible: true}, nil
	}
	_, err = s.Authorize(ctx, h, "unknown-result")
	assertStatus(t, err, 404)
	h.Del("X-API-Key")
	_, err = s.resolveCreation(ctx, h, op.ID, "historical")
	assertStatus(t, err, 409)
	if _, err = s.resolveCreation(ctx, h, op.ID, "unknown-result"); err != nil {
		t.Fatal(err)
	}
	n.p.User.ID, n.p.Memberships[0].Role = "viewer", "viewer"
	if _, err = s.Authorize(ctx, h, "unknown-result"); err != nil {
		t.Fatal("completed explicitly public KB stayed blocked", err)
	}
}

func TestCompletedCreationIndependentOfEarlierPendingOperation(t *testing.T) {
	s, _, n, h := setup("creator", "contributor")
	ctx := context.Background()
	if _, err := s.BeginCreate(ctx, h, "request-1", hashKey("one")); err != nil {
		t.Fatal(err)
	}
	n.list = []kbIdentity{{ID: "pending-one", TenantID: 7, CreatorID: "creator"}}
	if _, err := s.BeginCreate(ctx, h, "request-2", hashKey("two")); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteCreate(ctx, h, "request-2", "completed-two"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authorize(ctx, h, "completed-two"); err != nil {
		t.Fatal("older pending operation blocked completed binding", err)
	}
	_, err := s.Authorize(ctx, h, "pending-one")
	assertStatus(t, err, 404)
}

func TestCreationRejectsUnprovenNativeInventory(t *testing.T) {
	for _, raw := range []string{`{"success":true,"data":null}`, `{"success":true,"data":[],"total":1}`, `{"success":true,"data":{"items":[]}}`, `{"success":true,"data":[{"id":"foreign","tenant_id":8}]}`, `{"success":true,"data":[{"id":"same","tenant_id":7},{"id":"same","tenant_id":7}]}`} {
		s, m, n, h := setup("creator", "contributor")
		n.inventory = json.RawMessage(raw)
		_, err := s.BeginCreate(context.Background(), h, "request-1", hashKey("body"))
		assertStatus(t, err, 502)
		if len(m.creations) != 0 {
			t.Fatal("creation started without a complete inventory")
		}
	}
}
