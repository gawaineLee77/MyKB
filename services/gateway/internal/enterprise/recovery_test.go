package enterprise

import (
	"context"
	"encoding/json"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"net/http"
	"path/filepath"
	"testing"
)

func TestMemberKeyRecoveryRequiresCurrentOwnerAndExactScope(t *testing.T) {
	for _, tc := range []struct {
		name, role string
		full       bool
		want       string
	}{
		{"transferred owner", "owner", false, ""}, {"former owner", "admin", false, "install.owner_required"}, {"broad key", "owner", true, "install.key_scope_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bearerFile, keyFile := filepath.Join(dir, "bearer"), filepath.Join(dir, "key")
			if err := WriteSecret(bearerFile, "synthetic-owner"); err != nil {
				t.Fatal(err)
			}
			if err := WriteSecret(keyFile, "synthetic-key"); err != nil {
				t.Fatal(err)
			}
			store := &memoryStore{exists: true, i: Installation{Stage: "ready", AdminUserID: "original-admin", DefaultTenantID: 42, MemberKeyID: "1"}}
			u := &fakeUpstream{principal: weknora.Principal{User: &weknora.User{ID: "new-owner", IsActive: true}, Memberships: []weknora.Membership{{TenantID: 42, Role: tc.role}}}}
			u.call = func(m, p string, h http.Header, b any) (json.RawMessage, error) {
				if m != "GET" {
					t.Fatal("recovery mutated native ownership")
				}
				if p == "/api/v1/tenants/42/api-keys" {
					return json.Marshal(map[string]any{"data": []any{map[string]any{"id": 8, "api_key": "synthetic-key", "full_access": tc.full, "capabilities": []string{"manage_members"}}}})
				}
				if p != "/api/v1/tenants/42/members" || h.Get("X-API-Key") != "synthetic-key" {
					t.Fatal("wrong key or space")
				}
				return json.RawMessage(`{"success":true}`), nil
			}
			s := &Service{Store: store, Upstream: u}
			_, err := s.RepairMemberKey(context.Background(), bearerFile, keyFile, 8)
			if tc.want != "" {
				if PublicError(err) != tc.want {
					t.Fatalf("%v", err)
				}
				if store.i.MemberKeyID != "1" {
					t.Fatal("invalid recovery persisted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if store.i.MemberKeyID != "8" || store.i.AdminUserID != "original-admin" || store.i.DefaultTenantID != 42 {
				t.Fatal("recovery changed installation identity")
			}
			if _, err = s.RepairMemberKey(context.Background(), bearerFile, keyFile, 8); err != nil {
				t.Fatal(err)
			}
		})
	}
}
