package enterprise

import (
	"context"
	"encoding/json"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/identity"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
	"net/http"
	"sync"
	"testing"
	"time"
)

type employeeDirectory struct{ id identity.Identity }

func (d *employeeDirectory) GetByUpstreamEmail(context.Context, string) (identity.Identity, error) {
	return d.id, nil
}
func (d *employeeDirectory) BindLocalAccount(context.Context, string, string) error { return nil }
func (d *employeeDirectory) MemberAlias(context.Context, string) (string, error) {
	return d.id.UpstreamEmail, nil
}

func employeeFixture(t *testing.T) (*Service, *memoryStore, *fakeUpstream, *employeeDirectory) {
	t.Helper()
	secret := installInput(t)
	store := &memoryStore{exists: true, i: Installation{Stage: "ready", AdminUserID: "admin", DefaultTenantID: 42, MemberKeyRef: secret.PasswordFile}}
	upstream := &fakeUpstream{principal: weknora.Principal{User: &weknora.User{ID: "employee", Email: "alias@example.invalid", IsActive: true}}}
	dir := &employeeDirectory{id: identity.Identity{BrokerSubject: "subject", UpstreamEmail: "alias@example.invalid", Status: identity.StatusActive, LocalUserID: "employee"}}
	return &Service{Store: store, Upstream: upstream, Identities: dir}, store, upstream, dir
}

func TestEmployeeConcurrentOnboardingAddsExactlyOnceAndRemovalPersists(t *testing.T) {
	service, store, u, _ := employeeFixture(t)
	calls := 0
	u.call = func(m, p string, h http.Header, b any) (json.RawMessage, error) {
		calls++
		if m != "POST" || p != "/api/v1/tenants/42/members" || h.Get("X-API-Key") == "" {
			t.Error("wrong membership authority")
		}
		input := b.(map[string]string)
		if input["email"] != "alias@example.invalid" || input["role"] != "viewer" {
			t.Error("wrong default member")
		}
		u.principal.Memberships = []weknora.Membership{{TenantID: 42, Role: "viewer"}}
		return json.RawMessage(`{"success":true}`), nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			status, err := service.Onboarding(context.Background(), Bearer("employee"), true)
			if err != nil || status.State != "ready" {
				t.Errorf("%+v %v", status, err)
			}
		})
	}
	wg.Wait()
	if calls != 1 || store.onboard["subject"].CompletedAt == nil {
		t.Fatal("onboarding not durable or repeated")
	}
	u.principal.Memberships = nil
	for range 2 {
		status, err := service.Onboarding(context.Background(), Bearer("employee"), true)
		if err != nil || status.State != "removed" || status.Retryable {
			t.Fatal("removed member rejoined")
		}
	}
	if calls != 1 {
		t.Fatal("removal caused an add")
	}
	u.principal.Memberships = []weknora.Membership{{TenantID: 99, Role: "contributor"}}
	status, err := service.Onboarding(context.Background(), Bearer("employee"), false)
	if err != nil || status.State != "removed" || len(status.Memberships) != 1 {
		t.Fatal("other active workspace lost")
	}
}

func TestEmployeePreservesHigherRoleAndRecoversUnrecordedSuccess(t *testing.T) {
	for _, role := range []string{"contributor", "admin", "owner"} {
		t.Run(role, func(t *testing.T) {
			service, store, u, _ := employeeFixture(t)
			store.onboard = map[string]OnboardingRecord{"subject": {Subject: "subject", UserID: "employee", TenantID: 42, State: "joining", OutcomeUnknown: true}}
			u.principal.Memberships = []weknora.Membership{{TenantID: 42, Role: role}}
			u.call = func(string, string, http.Header, any) (json.RawMessage, error) {
				t.Fatal("overwrote existing membership")
				return nil, nil
			}
			status, err := service.Onboarding(context.Background(), Bearer("employee"), true)
			if err != nil || status.State != "ready" || status.Memberships[0].Role != role {
				t.Fatal("role not preserved")
			}
		})
	}
}

func TestEmployeeAmbiguousWriteDoesNotReaddAndDisabledIdentityRejected(t *testing.T) {
	service, store, u, dir := employeeFixture(t)
	calls := 0
	u.call = func(string, string, http.Header, any) (json.RawMessage, error) {
		calls++
		return nil, &weknora.Error{Code: "upstream.timeout", StatusCode: 502}
	}
	for range 2 {
		status, err := service.Onboarding(context.Background(), Bearer("employee"), true)
		if err != nil || status.State != "failed" || status.Retryable {
			t.Fatal("ambiguous write retried")
		}
	}
	if calls != 1 || !store.onboard["subject"].OutcomeUnknown {
		t.Fatal("missing durable ambiguity")
	}
	dir.id.Status = identity.StatusSuspended
	if _, err := service.Onboarding(context.Background(), Bearer("employee"), true); PublicError(err) != "identity.suspended" {
		t.Fatal("suspended employee accepted")
	}
	dir.id.Status = identity.StatusActive
	dir.id.LocalUserID = "someone-else"
	if _, err := service.Onboarding(context.Background(), Bearer("employee"), true); PublicError(err) != "identity.unlinked" {
		t.Fatal("identity mismatch accepted")
	}
}

func TestCompletedTimestampCannotBeOverwrittenByRetry(t *testing.T) {
	service, store, u, _ := employeeFixture(t)
	completed := time.Now().Add(-time.Hour)
	store.onboard = map[string]OnboardingRecord{"subject": {Subject: "subject", UserID: "employee", TenantID: 42, State: "ready", CompletedAt: &completed}}
	u.call = func(string, string, http.Header, any) (json.RawMessage, error) {
		t.Fatal("completed record called write")
		return nil, nil
	}
	_, err := service.Onboarding(context.Background(), Bearer("employee"), true)
	if err != nil || !store.onboard["subject"].CompletedAt.Equal(completed) {
		t.Fatal("completion changed")
	}
}
