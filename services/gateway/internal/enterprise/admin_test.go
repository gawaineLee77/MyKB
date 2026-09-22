package enterprise

import (
	"context"
	"encoding/json"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"net/http"
	"testing"
)

func TestAdminRefreshChecksActualSubjectAndRevokesMismatch(t *testing.T) {
	store := &memoryStore{exists: true, i: Installation{Stage: "account_ready", AdminEmail: "admin@example.invalid", AdminUserID: "admin"}}
	revoked := false
	upstream := &fakeUpstream{principal: weknora.Principal{User: &weknora.User{ID: "employee", IsActive: true}}}
	upstream.call = func(m, p string, h http.Header, input any) (json.RawMessage, error) {
		if p == "/api/v1/auth/logout" {
			revoked = true
			return json.RawMessage(`{"success":true}`), nil
		}
		if p != "/api/v1/auth/refresh" || h.Get("Authorization") != "" {
			t.Fatalf("unexpected request %s", p)
		}
		return json.RawMessage(`{"success":true,"access_token":"new","refresh_token":"new-refresh"}`), nil
	}
	service := &Service{Store: store, Upstream: upstream}
	input := json.RawMessage(`{"refreshToken":"employee-refresh"}`)
	if _, err := service.AdminAuth(context.Background(), "refresh", nil, input); PublicError(err) != "admin.authentication_failed" || !revoked {
		t.Fatal("employee refresh escaped admin boundary")
	}
	upstream.principal.User = &weknora.User{ID: "admin", IsActive: true, IsSystemAdmin: true}
	raw, err := service.AdminAuth(context.Background(), "refresh", nil, input)
	if err != nil {
		t.Fatal(err)
	}
	var result session
	_ = json.Unmarshal(raw, &result)
	if result.AccessToken != "new" || result.Token != "" {
		t.Fatal("refresh DTO was changed")
	}
	upstream.principal.User.IsActive = false
	if _, err := service.AdminAuth(context.Background(), "refresh", nil, input); err == nil {
		t.Fatal("disabled admin accepted")
	}
}

func TestAdminLoginRejectsEmployeeBeforeUpstream(t *testing.T) {
	service := &Service{Store: &memoryStore{exists: true, i: Installation{AdminEmail: "admin@example.invalid", AdminUserID: "admin"}}}
	_, err := service.AdminAuth(context.Background(), "login", nil, json.RawMessage(`{"email":"employee@example.invalid","password":"synthetic"}`))
	if PublicError(err) != "admin.authentication_failed" {
		t.Fatal("employee login accepted")
	}
}
