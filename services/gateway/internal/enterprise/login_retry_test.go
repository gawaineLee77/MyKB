package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type reissuedTokenStub struct {
	logins  int
	forever bool
}

func (s *reissuedTokenStub) EnterpriseRequest(_ context.Context, _ string, path string, _ http.Header, _ any) (json.RawMessage, error) {
	if path == "/api/v1/auth/login" {
		s.logins++
	}
	return json.RawMessage(`{"success":true,"token":"synthetic","refresh_token":"synthetic-refresh"}`), nil
}
func (s *reissuedTokenStub) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	if s.logins == 1 || s.forever {
		return weknora.Principal{}, &weknora.Error{StatusCode: 401, Code: "upstream.unauthorized"}
	}
	return weknora.Principal{User: &weknora.User{ID: "admin", IsActive: true, IsSystemAdmin: true}}, nil
}

func TestPasswordLoginRetriesReissuedRevokedTokenOnce(t *testing.T) {
	for _, forever := range []bool{false, true} {
		upstream := &reissuedTokenStub{forever: forever}
		s := &Service{Upstream: upstream, Store: &memoryStore{exists: true, i: Installation{AdminUserID: "admin", AdminEmail: "admin@example.invalid"}}}
		_, err := s.AdminAuth(context.Background(), "login", nil, json.RawMessage(`{"email":"admin@example.invalid","password":"synthetic"}`))
		if upstream.logins != 2 || (err != nil) != forever {
			t.Fatalf("logins=%d err=%v", upstream.logins, err)
		}
	}
}
