package grant

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

type ownerResolverStub struct {
	result ownership.Ownership
	err    error
}

func (s ownerResolverStub) Resolve(context.Context, string, http.Header) (ownership.Ownership, error) {
	return s.result, s.err
}

type memoryStore struct {
	grants      map[string]Grant
	createCalls int
}

type auditRecorderStub struct {
	events []audit.Event
	err    error
}

func (s *auditRecorderStub) Record(_ context.Context, event audit.Event) error {
	s.events = append(s.events, event)
	return s.err
}

type concurrentCreateStore struct {
	*memoryStore
	winner Grant
}

func (s *concurrentCreateStore) Create(_ context.Context, _ Grant) (Grant, error) {
	s.createCalls++
	s.grants[s.winner.ID] = s.winner
	return Grant{}, ErrConflict
}

func newMemoryStore(items ...Grant) *memoryStore {
	store := &memoryStore{grants: make(map[string]Grant)}
	for _, item := range items {
		store.grants[item.ID] = item
	}
	return store
}

func (s *memoryStore) Create(_ context.Context, candidate Grant) (Grant, error) {
	s.createCalls++
	for _, item := range s.grants {
		if item.KnowledgeBaseID == candidate.KnowledgeBaseID && item.SubjectType == candidate.SubjectType &&
			item.SubjectID == candidate.SubjectID && item.RevokedAt == nil {
			return Grant{}, ErrConflict
		}
	}
	s.grants[candidate.ID] = candidate
	return candidate, nil
}

func (s *memoryStore) Get(_ context.Context, id string) (Grant, error) {
	item, ok := s.grants[id]
	if !ok {
		return Grant{}, ErrNotFound
	}
	return item, nil
}

func (s *memoryStore) FindCurrentBySubject(_ context.Context, kbID string, subjectType SubjectType, subjectID string) (Grant, error) {
	for _, item := range s.grants {
		if item.KnowledgeBaseID == kbID && item.SubjectType == subjectType && item.SubjectID == subjectID && item.RevokedAt == nil {
			return item, nil
		}
	}
	return Grant{}, ErrNotFound
}

func (s *memoryStore) ListCurrentByKB(_ context.Context, kbID string) ([]Grant, error) {
	result := make([]Grant, 0)
	for _, item := range s.grants {
		if item.KnowledgeBaseID == kbID && item.RevokedAt == nil {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *memoryStore) EffectiveUserGrant(_ context.Context, kbID, userID string, at time.Time) (Grant, error) {
	item, err := s.FindCurrentBySubject(context.Background(), kbID, SubjectUser, userID)
	if err != nil || !item.Active(at) {
		return Grant{}, ErrNotFound
	}
	return item, nil
}

func (s *memoryStore) Update(_ context.Context, id string, expectedRevision int64, permission Permission, expiresAt *time.Time, correlationID string, at time.Time) (Grant, error) {
	item, ok := s.grants[id]
	if !ok || item.RevokedAt != nil || item.Revision != expectedRevision {
		return Grant{}, ErrRevisionConflict
	}
	item.Permission = permission
	item.ExpiresAt = expiresAt
	item.UpdatedAt = at
	item.Revision++
	item.LastAuditCorrelationID = correlationID
	s.grants[id] = item
	return item, nil
}

func (s *memoryStore) Revoke(_ context.Context, id string, expectedRevision int64, correlationID string, at time.Time) (Grant, error) {
	item, ok := s.grants[id]
	if !ok || item.RevokedAt != nil || item.Revision != expectedRevision {
		return Grant{}, ErrRevisionConflict
	}
	item.RevokedAt = &at
	item.UpdatedAt = at
	item.Revision++
	item.LastAuditCorrelationID = correlationID
	s.grants[id] = item
	return item, nil
}

func TestServiceCreateIsOwnerOnlyAndIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	service := mustService(t, store, ownerResolverStub{result: ragOwner()}, now)
	request := CreateRequest{
		SubjectType: SubjectUser, SubjectID: "bob", Permission: PermissionViewer, CorrelationID: "request-1",
	}
	created, err := service.Create(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "grant-fixed" || created.Revision != 1 || created.GrantedBy != "alice" || store.createCalls != 1 {
		t.Fatalf("created grant = %+v; calls=%d", created, store.createCalls)
	}
	retried, err := service.Create(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, request, nil)
	if err != nil || retried.ID != created.ID || store.createCalls != 1 {
		t.Fatalf("idempotent retry = %+v, %v; calls=%d", retried, err, store.createCalls)
	}
}

func TestServiceCreateAcceptsEquivalentConcurrentWinner(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	winner := Grant{
		ID: "grant-winner", KnowledgeBaseID: "kb-1", SubjectType: SubjectUser, SubjectID: "bob",
		Permission: PermissionViewer, GrantedBy: "alice", CreatedAt: now, UpdatedAt: now,
		Revision: 1, LastAuditCorrelationID: "other-request",
	}
	store := &concurrentCreateStore{memoryStore: newMemoryStore(), winner: winner}
	service := mustService(t, store, ownerResolverStub{result: ragOwner()}, now)
	created, err := service.Create(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, CreateRequest{
		SubjectType: SubjectUser, SubjectID: "bob", Permission: PermissionViewer, CorrelationID: "request-1",
	}, nil)
	if err != nil || created.ID != winner.ID || store.createCalls != 1 {
		t.Fatalf("concurrent Create() = %+v, %v; calls=%d", created, err, store.createCalls)
	}
}

func TestGrantActiveHonorsExpiryAndRevocation(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	expires := now.Add(time.Minute)
	item := Grant{ExpiresAt: &expires}
	if !item.Active(now) || item.Active(expires) {
		t.Fatalf("expiry boundary was not enforced")
	}
	revoked := now
	item.RevokedAt = &revoked
	if item.Active(now.Add(-time.Second)) {
		t.Fatal("revoked grant remained active")
	}
}

func TestServiceRecordsGrantLifecycleAudit(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	recorder := &auditRecorderStub{}
	service, err := NewService(store, ownerResolverStub{result: ragOwner()},
		WithClock(func() time.Time { return now }),
		WithIDGenerator(func() (string, error) { return "grant-1", nil }),
		WithAuditRecorder(recorder),
	)
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{UserID: "alice", TenantID: 42}
	created, err := service.Create(context.Background(), "kb-1", actor, CreateRequest{
		SubjectType: SubjectUser, SubjectID: "bob", Permission: PermissionViewer, CorrelationID: "request-1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(context.Background(), "kb-1", created.ID, actor, UpdateRequest{
		ExpectedRevision: 1, Permission: PermissionEditor, CorrelationID: "request-2",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Revoke(context.Background(), "kb-1", created.ID, actor, RevokeRequest{
		ExpectedRevision: updated.Revision, CorrelationID: "request-3",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 3 || recorder.events[0].Action != "grant.create" ||
		recorder.events[1].Action != "grant.update" || recorder.events[2].Action != "grant.revoke" ||
		len(recorder.events[1].OldValue) == 0 || len(recorder.events[1].NewValue) == 0 {
		t.Fatalf("audit events = %+v", recorder.events)
	}
}

func TestServiceRejectsUnsafeCreateRequests(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	tests := []struct {
		name    string
		owner   ownership.Ownership
		actor   Actor
		request CreateRequest
		want    error
	}{
		{
			name: "non owner", owner: ragOwner(), actor: Actor{UserID: "bob", TenantID: 42},
			request: CreateRequest{SubjectType: SubjectUser, SubjectID: "carol", Permission: PermissionViewer, CorrelationID: "r"},
			want:    ErrNotOwner,
		},
		{
			name: "personal notes", owner: ownership.Ownership{KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModePersonalNotes},
			actor:   Actor{UserID: "alice", TenantID: 42},
			request: CreateRequest{SubjectType: SubjectUser, SubjectID: "bob", Permission: PermissionViewer, CorrelationID: "r"},
			want:    ErrPersonalNotes,
		},
		{
			name: "group disabled", owner: ragOwner(), actor: Actor{UserID: "alice", TenantID: 42},
			request: CreateRequest{SubjectType: SubjectGroup, SubjectID: "group-1", Permission: PermissionViewer, CorrelationID: "r"},
			want:    ErrSubjectUnsupported,
		},
		{
			name: "self grant", owner: ragOwner(), actor: Actor{UserID: "alice", TenantID: 42},
			request: CreateRequest{SubjectType: SubjectUser, SubjectID: "alice", Permission: PermissionViewer, CorrelationID: "r"},
			want:    ErrInvalid,
		},
		{
			name: "past expiry", owner: ragOwner(), actor: Actor{UserID: "alice", TenantID: 42},
			request: CreateRequest{SubjectType: SubjectUser, SubjectID: "bob", Permission: PermissionViewer, ExpiresAt: &past, CorrelationID: "r"},
			want:    ErrInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryStore()
			service := mustService(t, store, ownerResolverStub{result: test.owner}, now)
			_, err := service.Create(context.Background(), "kb-1", test.actor, test.request, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
			if store.createCalls != 0 {
				t.Fatalf("unsafe request wrote %d grants", store.createCalls)
			}
		})
	}
}

func TestServiceListUpdateAndRevokeLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	existing := Grant{
		ID: "grant-1", KnowledgeBaseID: "kb-1", SubjectType: SubjectUser, SubjectID: "bob",
		Permission: PermissionViewer, GrantedBy: "alice", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
		Revision: 1, LastAuditCorrelationID: "request-1",
	}
	store := newMemoryStore(existing)
	service := mustService(t, store, ownerResolverStub{result: ragOwner()}, now)
	actor := Actor{UserID: "alice", TenantID: 42}
	items, err := service.List(context.Background(), "kb-1", actor, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("List() = %+v, %v", items, err)
	}
	updated, err := service.Update(context.Background(), "kb-1", "grant-1", actor, UpdateRequest{
		ExpectedRevision: 1, Permission: PermissionEditor, CorrelationID: "request-2",
	}, nil)
	if err != nil || updated.Permission != PermissionEditor || updated.Revision != 2 {
		t.Fatalf("Update() = %+v, %v", updated, err)
	}
	if _, err := service.Update(context.Background(), "kb-1", "grant-1", actor, UpdateRequest{
		ExpectedRevision: 1, Permission: PermissionViewer, CorrelationID: "stale",
	}, nil); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Update() error = %v", err)
	}
	revoked, err := service.Revoke(context.Background(), "kb-1", "grant-1", actor, RevokeRequest{
		ExpectedRevision: 2, CorrelationID: "request-3",
	}, nil)
	if err != nil || revoked.RevokedAt == nil || revoked.Revision != 3 {
		t.Fatalf("Revoke() = %+v, %v", revoked, err)
	}
	if _, err := store.EffectiveUserGrant(context.Background(), "kb-1", "bob", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked grant remained effective: %v", err)
	}
}

func ragOwner() ownership.Ownership {
	return ownership.Ownership{KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModeRAG}
}

func mustService(t *testing.T, store Store, owners OwnerResolver, now time.Time) *Service {
	t.Helper()
	service, err := NewService(store, owners,
		WithClock(func() time.Time { return now }),
		WithIDGenerator(func() (string, error) { return "grant-fixed", nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
