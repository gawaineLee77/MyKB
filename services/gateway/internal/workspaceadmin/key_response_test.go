package workspaceadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func encryptedCreateResponse(key *keyDTO) {
	// v0.8.0's GORM BeforeSave mutates only the returned api_key field.
	// The subsequent list runs AfterFind, yielding the stored plaintext.
	key.APIKey = "aes-gcm:synthetic-ciphertext"
}

func TestCreatedKeyReadbackUsesNativePlaintextAndPreservesAttribution(t *testing.T) {
	for _, actor := range []string{"owner", "admin"} {
		t.Run(actor, func(t *testing.T) {
			s, store, upstream := fixture(t)
			upstream.createResponse = encryptedCreateResponse
			s.KnowledgeStore = newKnowledgeMemory()
			raw, err := s.CreateKey(context.Background(), upstream.principals[actor], header(actor), 1, validInput())
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Data keyDTO `json:"data"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			key := response.Data
			if key.APIKey != "secret-101" || key.Token != key.APIKey || strings.Contains(string(raw), "ciphertext") {
				t.Fatal("create did not return only the verified plaintext credential")
			}
			stored := store.keys["1:101"]
			if stored.Fingerprint != fingerprint(key.APIKey) || stored.CreatorUserID != upstream.principals[actor].User.ID {
				t.Fatal("creation lost the real creator or fingerprinted ciphertext")
			}
			if _, err := s.MachineKnowledgePolicy(context.Background(), memberKeyHeader(key.APIKey, 1), 1, "kb-1"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreatedKeyReadbackRejectsContractMismatchAndCleansPending(t *testing.T) {
	cases := map[string]func(*keyDTO){
		"id":           func(k *keyDTO) { k.ID++ },
		"scope":        func(k *keyDTO) { k.ScopeType = "platform" },
		"name":         func(k *keyDTO) { k.Name = "wrong-marker" },
		"token":        func(k *keyDTO) { k.Token = "wrong-plaintext" },
		"no plaintext": func(k *keyDTO) { encryptedCreateResponse(k); k.Token = "" },
		"full access":  func(k *keyDTO) { k.FullAccess = true },
		"kb scope":     func(k *keyDTO) { k.KnowledgeBaseIDs = []string{"other-kb"} },
		"capabilities": func(k *keyDTO) { k.Capabilities = []string{"manage_members"} },
		"expiry":       func(k *keyDTO) { expiry := time.Now().Add(time.Hour); k.ExpiresAt = &expiry },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			s, store, upstream := fixture(t)
			upstream.createResponse = change
			raw, err := s.CreateKey(context.Background(), upstream.principals["admin"], header("admin"), 1, validInput())
			if err == nil || len(raw) != 0 || len(store.keys) != 0 || len(upstream.keys) != 0 || len(store.pending[1]) != 0 {
				t.Fatalf("mismatched response escaped or left an active pending key: %v", err)
			}
		})
	}
}

func TestCreatedKeyReadbackCannotAttributeExistingProtectedKey(t *testing.T) {
	s, store, upstream := fixture(t)
	existing := keyDTO{ID: 99, ScopeType: "tenant", Name: "mindcreek-internal:existing",
		APIKey: "protected-secret", Token: "protected-secret", KnowledgeBaseIDs: []string{"kb-1"}, Capabilities: []string{"retrieve"}}
	upstream.keys[existing.ID] = existing
	// Both the POST response and the following GET agree on an existing key.
	// Its name must still match this request's unguessable creation marker.
	upstream.createResponse = func(key *keyDTO) { *key = existing }
	raw, err := s.CreateKey(context.Background(), upstream.principals["admin"], header("admin"), 1, validInput())
	if err == nil || len(raw) != 0 || len(store.keys) != 0 || len(store.pending[1]) != 0 {
		t.Fatal("an existing protected key was attributed or exposed")
	}
	if len(upstream.keys) != 1 || upstream.keys[99].APIKey != existing.APIKey {
		t.Fatal("recovery did not remove only the newly created pending key")
	}
}

func TestInternalMemberKeyReadbackPersistsPlaintextAndStaysProtected(t *testing.T) {
	s, store, upstream := memberFixture(t)
	upstream.createResponse = encryptedCreateResponse
	_, err := s.Manage(context.Background(), upstream.principals["admin"], header("admin"), 1,
		http.MethodPost, "/members", json.RawMessage(`{"email":"new@example.test","role":"viewer"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(upstream.writes) != 1 || upstream.writes[0].header.Get("X-API-Key") != "secret-101" {
		t.Fatal("member write did not use the verified plaintext credential")
	}
	plain, err := s.open(1, "member-key", store.delegations[1].MemberKeyCipher)
	if err != nil || string(plain) != "secret-101" {
		t.Fatal("member delegation persisted the native ciphertext")
	}
	listed, err := s.ListKeys(context.Background(), upstream.principals["owner"], header("owner"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listed), "secret-101") || strings.Contains(string(listed), "ciphertext") {
		t.Fatal("protected member key leaked through ordinary key listing")
	}
}

func TestInternalKnowledgeKeyReadbackPersistsPlaintextAndStaysProtected(t *testing.T) {
	s, _, upstream := memberFixture(t)
	store := newKnowledgeMemory()
	s.KnowledgeStore = store
	upstream.createResponse = encryptedCreateResponse
	forward, err := s.executionHeaders(context.Background(), 1, "kb-1", header("owner"))
	if err != nil {
		t.Fatal(err)
	}
	if forward.Get("X-API-Key") != "secret-101" || len(store.keys[1]) != 1 {
		t.Fatal("knowledge execution did not use the verified plaintext credential")
	}
	plain, err := s.open(1, "knowledge-key:kb-1", store.keys[1][0].Cipher)
	if err != nil || string(plain) != "secret-101" {
		t.Fatal("knowledge execution persisted the native ciphertext")
	}
	listed, err := s.ListKeys(context.Background(), upstream.principals["owner"], header("owner"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(listed), "secret-101") || strings.Contains(string(listed), "ciphertext") {
		t.Fatal("protected knowledge key leaked through ordinary key listing")
	}
}
