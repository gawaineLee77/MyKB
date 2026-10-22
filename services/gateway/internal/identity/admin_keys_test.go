package identity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSuspensionAndReactivationInvalidateCreatorKeys(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	id, err := store.Upsert(ctx, Claims{Issuer: "https://synthetic.invalid", Subject: "user", CorporateEmail: "user@synthetic.invalid", Username: "user"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindLocalPrincipal(ctx, id.UpstreamEmail, "employee", 42); err != nil {
		t.Fatal(err)
	}
	admin, _ := NewAdminService(store)
	admin.WithStatusLock = func(_ context.Context, _ string, fn func() error) error { return fn() }
	calls := 0
	admin.DisableKeys = func(ctx context.Context, user string) error {
		calls++
		if user != "employee" {
			t.Fatal("wrong key owner")
		}
		current, _ := store.GetByBrokerSubject(ctx, id.BrokerSubject)
		if current.Status != StatusSuspended {
			t.Fatal("key invalidation must occur while identity is suspended")
		}
		return nil
	}
	if _, err := admin.ChangeStatus(ctx, id.BrokerSubject, StatusSuspended, "operator", "suspend-test", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ChangeStatus(ctx, id.BrokerSubject, StatusActive, "operator", "activate-test", ""); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("revocation calls = %d", calls)
	}
	if _, err := admin.ChangeStatus(ctx, id.BrokerSubject, StatusActive, "operator", "unchanged-test", ""); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("unchanged activation should not revoke new keys")
	}
}

func TestFailedKeyInvalidationKeepsIdentitySuspended(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	id, _ := store.Upsert(ctx, Claims{Issuer: "https://synthetic.invalid", Subject: "user", CorporateEmail: "user@synthetic.invalid", Username: "user"}, time.Now())
	_ = store.BindLocalPrincipal(ctx, id.UpstreamEmail, "employee", 42)
	admin, _ := NewAdminService(store)
	admin.WithStatusLock = func(_ context.Context, _ string, fn func() error) error { return fn() }
	admin.DisableKeys = func(context.Context, string) error { return errors.New("synthetic outage") }
	if _, err := admin.ChangeStatus(ctx, id.BrokerSubject, StatusSuspended, "operator", "suspend-test", ""); err == nil {
		t.Fatal("expected invalidation failure")
	}
	if _, err := admin.ChangeStatus(ctx, id.BrokerSubject, StatusActive, "operator", "activate-test", ""); err == nil {
		t.Fatal("activation must fail closed")
	}
	current, _ := store.GetByBrokerSubject(ctx, id.BrokerSubject)
	if current.Status != StatusSuspended {
		t.Fatal("identity reactivated without key invalidation")
	}
}

type statusRaceStore struct {
	*memoryStore
	readActive chan struct{}
	resume     chan struct{}
	once       sync.Once
}

func (s *statusRaceStore) GetByBrokerSubject(ctx context.Context, subject string) (Identity, error) {
	value, err := s.memoryStore.GetByBrokerSubject(ctx, subject)
	s.once.Do(func() {
		close(s.readActive)
		select {
		case <-s.resume:
		case <-ctx.Done():
		}
	})
	return value, err
}

func TestStatusLockPreventsStaleActivationAfterFailedSuspension(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	store := &statusRaceStore{memoryStore: &memoryStore{}, readActive: make(chan struct{}), resume: make(chan struct{})}
	id, err := store.Upsert(ctx, Claims{Issuer: "https://synthetic.invalid", Subject: "race", CorporateEmail: "race@synthetic.invalid", Username: "race"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindLocalPrincipal(ctx, id.UpstreamEmail, "employee", 42); err != nil {
		t.Fatal(err)
	}
	var lock sync.Mutex
	lockAttempts := make(chan string, 2)
	withLock := func(_ context.Context, subject string, fn func() error) error {
		lockAttempts <- subject
		lock.Lock()
		defer lock.Unlock()
		return fn()
	}
	// Two service instances model separate gateway processes sharing one
	// distributed lock. A read-active activation must finish before another
	// instance can suspend, even when the latter's key invalidation fails.
	activate, _ := NewAdminService(store)
	suspend, _ := NewAdminService(store)
	activate.WithStatusLock, suspend.WithStatusLock = withLock, withLock
	activate.DisableKeys = func(context.Context, string) error { return errors.New("activation should be a no-op") }
	suspend.DisableKeys = func(context.Context, string) error { return errors.New("synthetic credential database outage") }
	activationDone := make(chan error, 1)
	go func() {
		_, err := activate.ChangeStatus(ctx, " "+id.BrokerSubject+" ", StatusActive, "operator", "activate-race", "")
		activationDone <- err
	}()
	select {
	case <-store.readActive:
	case <-ctx.Done():
		t.Fatal("activation did not reach its active-state read")
	}
	if subject := <-lockAttempts; subject != id.BrokerSubject {
		t.Fatal("lock did not receive canonical broker subject")
	}
	suspensionDone := make(chan error, 1)
	go func() {
		_, err := suspend.ChangeStatus(ctx, id.BrokerSubject, StatusSuspended, "operator", "suspend-race", "")
		suspensionDone <- err
	}()
	select {
	case <-lockAttempts:
	case <-ctx.Done():
		t.Fatal("suspension did not attempt serialization")
	}
	close(store.resume)
	if err := <-activationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-suspensionDone; err == nil {
		t.Fatal("expected key invalidation failure")
	}
	current, err := store.memoryStore.GetByBrokerSubject(ctx, id.BrokerSubject)
	if err != nil || current.Status != StatusSuspended {
		t.Fatal("stale activation revived identity after failed invalidation", current.Status, err)
	}
}

func TestStatusMutationFailsClosedWithoutCredentialSerialization(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	id, _ := store.Upsert(ctx, Claims{Issuer: "https://synthetic.invalid", Subject: "unlocked", CorporateEmail: "unlocked@synthetic.invalid", Username: "unlocked"}, time.Now())
	admin, _ := NewAdminService(store)
	admin.DisableKeys = func(context.Context, string) error { return nil }
	if _, err := admin.ChangeStatus(ctx, id.BrokerSubject, StatusSuspended, "operator", "unlocked-test", ""); err == nil {
		t.Fatal("credential lifecycle configuration accepted missing serialization")
	}
	current, _ := store.GetByBrokerSubject(ctx, id.BrokerSubject)
	if current.Status != StatusActive || len(store.audits) != 0 {
		t.Fatal("failed prerequisite mutated identity or emitted a success audit")
	}
}
