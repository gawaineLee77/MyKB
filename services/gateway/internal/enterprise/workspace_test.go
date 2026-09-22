package enterprise

import (
	"context"
	"encoding/json"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"net/http"
	"path/filepath"
	"testing"
)

func TestDefaultWorkspacePersistsBeforeExternalWritesAndNeverReclaims(t *testing.T) {
	input := installInput(t)
	store := &memoryStore{exists: true, i: Installation{Stage: "account_ready", AdminEmail: input.Email, AdminUserID: "admin"}}
	creates, keys := 0, 0
	upstream := &fakeUpstream{principal: weknora.Principal{User: &weknora.User{ID: "admin", IsActive: true, IsSystemAdmin: true}, Memberships: []weknora.Membership{{TenantID: 42, Role: "owner"}}}}
	upstream.call = func(m, p string, h http.Header, body any) (json.RawMessage, error) {
		switch p {
		case "/api/v1/auth/login":
			return json.RawMessage(`{"success":true,"token":"a","refresh_token":"r"}`), nil
		case "/api/v1/tenants":
			creates++
			if store.i.Stage != "space_creating" {
				t.Fatal("create without durable marker")
			}
			return json.RawMessage(`{"success":true,"data":{"id":42}}`), nil
		case "/api/v1/tenants/42/api-keys":
			keys++
			if store.i.Stage != "key_creating" {
				t.Fatal("key without durable marker")
			}
			return json.RawMessage(`{"success":true,"data":{"id":8,"token":"synthetic-scoped-key"}}`), nil
		default:
			return json.RawMessage(`{"success":true,"data":{"members":[]}}`), nil
		}
	}
	service := &Service{Store: store, Upstream: upstream}
	space := WorkspaceInput{Name: "Company", PasswordFile: input.PasswordFile, MemberKeyFile: filepath.Join(t.TempDir(), "member-key")}
	result, err := service.DefaultWorkspace(context.Background(), space)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stage != "ready" || result.DefaultTenantID != 42 || result.MemberKeyID != "8" {
		t.Fatalf("bad state %+v", result)
	}
	upstream.principal.Memberships = nil
	space.PasswordFile = "missing"
	if _, err = service.DefaultWorkspace(context.Background(), space); err != nil {
		t.Fatal(err)
	}
	if creates != 1 || keys != 1 {
		t.Fatal("repeated installation changed ownership or keys")
	}
}

func TestUnknownSpaceCreationRequiresOwnedExplicitID(t *testing.T) {
	input := installInput(t)
	store := &memoryStore{exists: true, i: Installation{Stage: "space_creating", AdminEmail: input.Email, AdminUserID: "admin"}}
	upstream := &fakeUpstream{principal: weknora.Principal{User: &weknora.User{ID: "admin", IsActive: true, IsSystemAdmin: true}}}
	upstream.call = func(m, p string, h http.Header, b any) (json.RawMessage, error) {
		if p == "/api/v1/tenants" {
			t.Fatal("retried ambiguous create")
		}
		return json.RawMessage(`{"success":true,"token":"a","refresh_token":"r"}`), nil
	}
	service := &Service{Store: store, Upstream: upstream}
	space := WorkspaceInput{PasswordFile: input.PasswordFile}
	if _, err := service.DefaultWorkspace(context.Background(), space); PublicError(err) != "install.space_result_unknown" {
		t.Fatalf("%v", err)
	}
	space.AdoptTenantID = 99
	if _, err := service.DefaultWorkspace(context.Background(), space); PublicError(err) != "install.space_adoption_denied" {
		t.Fatal("unowned space adopted")
	}
}
