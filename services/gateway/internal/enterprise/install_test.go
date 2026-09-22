package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type memoryStore struct {
	mu      sync.Mutex
	i       Installation
	exists  bool
	saveErr bool
	onboard map[string]OnboardingRecord
}

func (m *memoryStore) Onboarding(_ context.Context, subject string) (OnboardingRecord, error) {
	o, ok := m.onboard[subject]
	if !ok {
		return o, ErrNotFound
	}
	return o, nil
}
func (m *memoryStore) SaveOnboarding(_ context.Context, o OnboardingRecord) error {
	if m.saveErr {
		return errors.New("save failed")
	}
	if m.onboard == nil {
		m.onboard = map[string]OnboardingRecord{}
	}
	m.onboard[o.Subject] = o
	return nil
}

func (m *memoryStore) Installation(context.Context) (Installation, error) {
	if !m.exists {
		return Installation{}, ErrNotFound
	}
	return m.i, nil
}
func (m *memoryStore) SaveInstallation(_ context.Context, i Installation) error {
	if m.saveErr {
		return errors.New("database unavailable")
	}
	m.i = i
	m.exists = true
	return nil
}
func (m *memoryStore) WithLock(_ context.Context, _ string, fn func(Store) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(m)
}

type fakeUpstream struct {
	mu        sync.Mutex
	call      func(string, string, http.Header, any) (json.RawMessage, error)
	principal weknora.Principal
}

func (u *fakeUpstream) EnterpriseRequest(_ context.Context, m, p string, h http.Header, b any) (json.RawMessage, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.call(m, p, h, b)
}
func (u *fakeUpstream) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.principal, nil
}

func installInput(t *testing.T) InstallInput {
	t.Helper()
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("Synthetic-only!42\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return InstallInput{Email: "admin@example.invalid", Username: "admin", PasswordFile: path}
}

func TestInstallationPrepareResumeAndBootstrap(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	input := installInput(t)
	registrations, settings := 0, 0
	upstream := &fakeUpstream{principal: weknora.Principal{User: &weknora.User{ID: "admin-id", Email: input.Email, IsActive: true}}}
	upstream.call = func(m, p string, h http.Header, b any) (json.RawMessage, error) {
		switch p {
		case "/api/v1/auth/register":
			if len(b.(map[string]string)) == 0 {
				return nil, &weknora.Error{UpstreamStatus: 403, StatusCode: 403}
			}
			registrations++
			return json.RawMessage(`{"success":true,"user":{"id":"admin-id","tenant_id":0}}`), nil
		case "/api/v1/auth/login":
			return json.RawMessage(`{"success":true,"token":"access","refresh_token":"refresh"}`), nil
		case "/api/v1/auth/logout":
			return json.RawMessage(`{"success":true}`), nil
		default:
			settings++
			return json.RawMessage(`{"value":true}`), nil
		}
	}
	service := &Service{Store: store, Upstream: upstream}
	for range 2 {
		if _, err := service.Prepare(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if registrations != 1 || store.i.Stage != "admin_registered" {
		t.Fatalf("duplicate registration or wrong stage: %d %s", registrations, store.i.Stage)
	}
	if _, err := service.ConfirmAdmin(ctx, input); PublicError(err) != "install.bootstrap_required_or_conflict" {
		t.Fatalf("unpromoted user accepted: %v", err)
	}
	upstream.principal.User.IsSystemAdmin = true
	if _, err := service.ConfirmAdmin(ctx, input); err != nil {
		t.Fatal(err)
	}
	if settings != 0 || store.i.Stage != "account_ready" {
		t.Fatalf("settings=%d stage=%s", settings, store.i.Stage)
	}
	input.PasswordFile = "missing-on-repeat"
	if _, err := service.ConfirmAdmin(ctx, input); err != nil {
		t.Fatal(err)
	}
	if settings != 0 {
		t.Fatal("repeated installation mutated settings")
	}
}

func TestAmbiguousRegistrationRequiresExplicitRecovery(t *testing.T) {
	input := installInput(t)
	store := &memoryStore{}
	calls := 0
	upstream := &fakeUpstream{call: func(string, string, http.Header, any) (json.RawMessage, error) {
		calls++
		return nil, errors.New("connection lost")
	}}
	service := &Service{Store: store, Upstream: upstream}
	for range 2 {
		if _, err := service.Prepare(context.Background(), input); PublicError(err) != "install.account_result_unknown" {
			t.Fatalf("got %v", err)
		}
	}
	if calls != 1 || store.i.Stage != "registering" {
		t.Fatal("ambiguous registration retried")
	}
	upstream.principal = weknora.Principal{User: &weknora.User{ID: "recovered", Email: input.Email}}
	upstream.call = func(string, string, http.Header, any) (json.RawMessage, error) {
		return json.RawMessage(`{"success":true,"token":"a","refresh_token":"r"}`), nil
	}
	input.AdoptAdminID = "wrong"
	if _, err := service.Prepare(context.Background(), input); PublicError(err) != "install.account_conflict" {
		t.Fatal("wrong identity adopted")
	}
	input.AdoptAdminID = "recovered"
	if _, err := service.Prepare(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if store.i.AdminUserID != "recovered" {
		t.Fatal("identity not persisted")
	}
}

func TestInstallationRejectsUnsafeSecretAndConflictingIdentity(t *testing.T) {
	input := installInput(t)
	if err := os.Chmod(input.PasswordFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(input.PasswordFile); PublicError(err) != "install.secret_invalid" {
		t.Fatal("read world-readable password")
	}
	service := &Service{Store: &memoryStore{exists: true, i: Installation{Stage: "ready", AdminEmail: "someone@example.invalid"}}}
	if _, err := service.Prepare(context.Background(), input); PublicError(err) != "install.account_conflict" {
		t.Fatal("foreign installation accepted")
	}
	if got := PublicError(errors.New("secret database credentials")); got != "enterprise.operation_failed" {
		t.Fatal("sensitive error exposed")
	}
}
