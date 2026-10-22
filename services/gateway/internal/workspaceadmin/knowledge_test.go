package workspaceadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type knowledgeMemory struct {
	delegations map[uint64]Delegation
	keys        map[uint64][]ExecutionKey
	policies    map[string]MachinePolicy
}

func newKnowledgeMemory() *knowledgeMemory {
	return &knowledgeMemory{map[uint64]Delegation{}, map[uint64][]ExecutionKey{}, map[string]MachinePolicy{}}
}
func (m *knowledgeMemory) ExecutionDelegation(_ context.Context, id uint64) (Delegation, error) {
	d, ok := m.delegations[id]
	if !ok {
		return d, ErrNotFound
	}
	return d, nil
}
func (m *knowledgeMemory) SaveExecutionDelegation(_ context.Context, d Delegation) error {
	m.delegations[d.TenantID] = d
	return nil
}
func (m *knowledgeMemory) DeleteExecutionDelegation(_ context.Context, id uint64) error {
	delete(m.delegations, id)
	return nil
}
func (m *knowledgeMemory) ExecutionKeys(_ context.Context, id uint64) ([]ExecutionKey, error) {
	return m.keys[id], nil
}
func (m *knowledgeMemory) SaveExecutionKey(_ context.Context, k ExecutionKey) error {
	m.keys[k.TenantID] = append(m.keys[k.TenantID], k)
	return nil
}
func (m *knowledgeMemory) DeleteExecutionKeys(_ context.Context, id uint64) error {
	delete(m.keys, id)
	return nil
}
func (m *knowledgeMemory) MachinePolicy(_ context.Context, id, key uint64) (MachinePolicy, error) {
	p, ok := m.policies[keyRef(id, key)]
	if !ok {
		return p, ErrNotFound
	}
	return p, nil
}
func (m *knowledgeMemory) SaveMachinePolicy(_ context.Context, p MachinePolicy) error {
	m.policies[keyRef(p.TenantID, p.KeyID)] = p
	return nil
}
func (m *knowledgeMemory) DeleteMachinePolicy(_ context.Context, id, key uint64) error {
	delete(m.policies, keyRef(id, key))
	return nil
}

func TestMachineKnowledgeMappingIsExplicitAndIndependent(t *testing.T) {
	s, store, up := fixture(t)
	s.KnowledgeStore = newKnowledgeMemory()
	ctx := context.Background()
	input := json.RawMessage(`{"name":"Scoped","full_access":false,"knowledge_base_ids":["kb-1"],"capabilities":["retrieve"],"knowledge_operations":["document.download"]}`)
	raw, err := s.CreateKey(ctx, up.principals["admin"], header("admin"), 1, input)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data keyDTO `json:"data"`
	}
	_ = json.Unmarshal(raw, &response)
	key := response.Data
	if !key.KnowledgeMapped || len(key.KnowledgeOperations) != 1 {
		t.Fatal(key)
	}
	h := http.Header{"X-Api-Key": []string{key.APIKey}}
	policy, err := s.MachineKnowledgePolicy(ctx, h, 1, "kb-1")
	if err != nil || len(policy.Operations) != 1 || policy.AllKnowledgeBases {
		t.Fatal(policy, err)
	}
	if _, err := s.MachineKnowledgePolicy(ctx, h, 1, "kb-2"); err == nil {
		t.Fatal("key expanded beyond KB scope")
	}
	if _, err := s.MachineKnowledgePolicy(ctx, http.Header{"X-Api-Key": []string{"legacy"}}, 1, "kb-1"); err == nil {
		t.Fatal("legacy key inherited Owner access")
	}
	if err := store.DisableUserKeys(ctx, 1, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MachineKnowledgePolicy(ctx, h, 1, "kb-1"); err == nil {
		t.Fatal("revoked creator key remained usable")
	}
}
func TestOwnerAllKBRequiresSeparateExplicitSelection(t *testing.T) {
	s, _, up := fixture(t)
	s.KnowledgeStore = newKnowledgeMemory()
	ctx := context.Background()
	raw, err := s.CreateKey(ctx, up.principals["owner"], header("owner"), 1, json.RawMessage(`{"name":"Owner key","full_access":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data keyDTO `json:"data"`
	}
	_ = json.Unmarshal(raw, &response)
	if _, err := s.MachineKnowledgePolicy(ctx, http.Header{"X-Api-Key": []string{response.Data.APIKey}}, 1, "private"); err == nil {
		t.Fatal("native full_access implied product private KB access")
	}
	_, err = s.CreateKey(ctx, up.principals["admin"], header("admin"), 1, json.RawMessage(`{"name":"Bad","knowledge_base_ids":["kb-1"],"capabilities":["retrieve"],"all_knowledge_bases":true}`))
	assertCode(t, err, "kb_acl.key_all_scope_owner_required")
	var input keyInput
	_ = json.Unmarshal(json.RawMessage(`{"name":"All","full_access":true,"all_knowledge_bases":true,"knowledge_operations":["chunk.export"]}`), &input)
	native, _ := json.Marshal(input.native())
	if strings.Contains(string(native), "knowledge_operations") || strings.Contains(string(native), "all_knowledge_bases") {
		t.Fatal("product-only grants sent upstream")
	}
}
func TestContentDelegationRequiresDistinctOwnerAuthorization(t *testing.T) {
	s, store, up := fixture(t)
	content := newKnowledgeMemory()
	s.KnowledgeStore = content
	ctx := context.Background()
	if _, err := s.executionOwnerHeaders(ctx, 1); err == nil {
		t.Fatal("member delegation implicitly enabled content writes")
	}
	_, err := s.SetKnowledgeDelegation(ctx, up.principals["admin"], header("admin"), 1)
	assertCode(t, err, "workspace_admin.owner_required")
	status, err := s.SetKnowledgeDelegation(ctx, up.principals["owner"], header("owner"), 1)
	if err != nil || !status.Authorized {
		t.Fatal(status, err)
	}
	if _, err := s.executionOwnerHeaders(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeKnowledgeDelegation(ctx, up.principals["owner"], header("owner"), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.executionOwnerHeaders(ctx, 1); err == nil {
		t.Fatal("revocation did not disable content delegation")
	}
	if len(store.delegations[1].SessionCipher) == 0 {
		t.Fatal("revoking content delegation erased workspace delegation")
	}
}

type pausedDelegationCheck struct {
	*fakeUpstream
	entered chan struct{}
	resume  chan struct{}
}

func (f *pausedDelegationCheck) CurrentPrincipal(ctx context.Context, h http.Header) (weknora.Principal, error) {
	if h.Get("Authorization") == "Bearer delegated" {
		close(f.entered)
		select {
		case <-f.resume:
			return weknora.Principal{}, &weknora.Error{UpstreamStatus: 401}
		case <-ctx.Done():
			return weknora.Principal{}, ctx.Err()
		}
	}
	return f.fakeUpstream.CurrentPrincipal(ctx, h)
}

func TestMachinePolicyReadCannotResurrectConcurrentDelegationRevocation(t *testing.T) {
	s, store, upstream := fixture(t)
	s.KnowledgeStore = newKnowledgeMemory()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := s.CreateKey(ctx, upstream.principals["admin"], header("admin"), 1, validInput())
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data keyDTO `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	// A stale session could otherwise refresh successfully and SaveDelegation
	// after the concurrent Owner revoke has already deleted that delegation.
	upstream.refreshAccess = "renewed"
	upstream.principals["renewed"] = principal("o", "owner", 1)
	paused := &pausedDelegationCheck{upstream, make(chan struct{}), make(chan struct{})}
	s.Upstream = paused
	done := make(chan error, 1)
	go func() {
		_, err := s.MachineKnowledgePolicy(ctx, http.Header{"X-Api-Key": []string{response.Data.APIKey}}, 1, "kb-1")
		done <- err
	}()
	select {
	case <-paused.entered:
	case <-ctx.Done():
		t.Fatal("machine policy did not reach delegated session validation")
	}
	if err := s.RevokeDelegation(ctx, upstream.principals["owner"], header("owner"), 1); err != nil {
		t.Fatal(err)
	}
	close(paused.resume)
	select {
	case err := <-done:
		assertCode(t, err, "workspace_admin.delegation_expired")
	case <-ctx.Done():
		t.Fatal("machine policy validation did not finish")
	}
	if _, ok := store.delegations[1]; ok {
		t.Fatal("a read restored a revoked Owner delegation")
	}
	for _, call := range upstream.calls {
		if call == "POST /api/v1/auth/refresh" {
			t.Fatal("machine policy read refreshed delegation outside the workspace lock")
		}
	}
}
