package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type previewDirectory struct{ employeeDirectory }

func (previewDirectory) MemberAlias(_ context.Context, email string) (string, error) {
	switch email {
	case "existing@example.invalid":
		return "native-alias@example.invalid", nil
	case "new@example.invalid":
		return "new-alias@example.invalid", nil
	default:
		return "", identity.ErrNotFound
	}
}

func TestMemberPreviewOwnerOnlyAndPreservesNativeRole(t *testing.T) {
	for _, role := range []string{"owner", "admin", "contributor", "viewer"} {
		t.Run(role, func(t *testing.T) {
			calls := 0
			upstream := &fakeUpstream{call: func(method, path string, h http.Header, body any) (json.RawMessage, error) {
				calls++
				if method != "GET" || !strings.HasPrefix(path, "/api/v1/tenants/42/members?") || h.Get("Authorization") != "Bearer human" || h.Get("X-API-Key") != "" {
					t.Fatal("preview changed authority or performed a write")
				}
				return json.RawMessage(`{"success":true,"data":{"members":[{"user_id":"existing","email":"native-alias@example.invalid","role":"owner"}],"total":1}}`), nil
			}}
			s := &Service{Store: &memoryStore{i: Installation{AdminEmail: "admin@example.invalid"}, exists: true}, Upstream: upstream, Identities: &previewDirectory{}}
			p := weknora.Principal{User: &weknora.User{ID: "caller", IsActive: true}, Tenant: &weknora.Tenant{ID: 42}, Memberships: []weknora.Membership{{TenantID: 42, Role: role}}}
			rows, err := s.PreviewMembers(context.Background(), p, Bearer("human"), []string{"EXISTING@example.invalid", "new@example.invalid", "unknown@example.invalid", "invalid"})
			if role != "owner" {
				if err == nil || calls != 0 {
					t.Fatal("non-Owner performed a directory lookup")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rows[0].Role != "owner" || rows[0].State != "existing" || rows[1].State != "ready" || rows[2].State != "unregistered" || rows[3].State != "invalid" {
				t.Fatalf("unexpected preview %#v", rows)
			}
			p.Memberships[0].TenantID = 99
			if _, err := s.PreviewMembers(context.Background(), p, Bearer("human"), []string{"new@example.invalid"}); err == nil {
				t.Fatal("cross-space Owner accepted")
			}
			p.Memberships[0].TenantID = 42
			h := Bearer("human")
			h.Set("X-API-Key", "machine")
			if _, err := s.PreviewMembers(context.Background(), p, h, []string{"new@example.invalid"}); err == nil {
				t.Fatal("machine inherited Owner")
			}
		})
	}
}
