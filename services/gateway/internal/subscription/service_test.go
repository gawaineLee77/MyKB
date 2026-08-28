package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
)

type subscriptionStoreStub struct {
	value     Subscription
	createErr error
	missFirst bool
	readCount int
}

func (s *subscriptionStoreStub) Create(_ context.Context, value Subscription) (Subscription, error) {
	if s.createErr != nil {
		return Subscription{}, s.createErr
	}
	s.value = value
	return value, nil
}
func (s *subscriptionStoreStub) GetByPublicationUser(_ context.Context, publicationID, userID string) (Subscription, error) {
	s.readCount++
	if s.missFirst && s.readCount == 1 {
		return Subscription{}, ErrNotFound
	}
	if s.value.PublicationID != publicationID || s.value.SubscriberID != userID {
		return Subscription{}, ErrNotFound
	}
	return s.value, nil
}
func (s *subscriptionStoreStub) ListByUser(context.Context, string) ([]Subscription, error) {
	return []Subscription{s.value}, nil
}
func (s *subscriptionStoreStub) Activate(_ context.Context, id string, tenant uint64, revision int64, correlation string, at time.Time) (Subscription, error) {
	s.value.Status = StatusActive
	s.value.SubscriberTenantID = tenant
	s.value.LastSeenRevision = revision
	s.value.EndedAt = nil
	s.value.UpdatedAt = at
	s.value.LastAuditCorrelationID = correlation
	return s.value, nil
}
func (s *subscriptionStoreStub) Unsubscribe(_ context.Context, id, correlation string, at time.Time) (Subscription, error) {
	s.value.Status = StatusUnsubscribed
	s.value.EndedAt = &at
	s.value.UpdatedAt = at
	s.value.LastAuditCorrelationID = correlation
	return s.value, nil
}
func (s *subscriptionStoreStub) MarkSeen(_ context.Context, id string, revision int64, correlation string, at time.Time) (Subscription, error) {
	s.value.LastSeenRevision = revision
	s.value.UpdatedAt = at
	return s.value, nil
}
func (s *subscriptionStoreStub) InactivatePublication(context.Context, string, string, time.Time) error {
	return nil
}
func (s *subscriptionStoreStub) InactivateOutsideAudience(context.Context, string, publication.Audience, string, time.Time) error {
	return nil
}

type publicationReaderStub struct{ value publication.Publication }

func (s publicationReaderStub) Get(context.Context, string) (publication.Publication, error) {
	return s.value, nil
}

type revisionReaderStub struct{ value int64 }

func (s revisionReaderStub) Current(context.Context, string) (int64, error) { return s.value, nil }

func TestSubscribeIsIdempotentAndTracksUpdates(t *testing.T) {
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	pub := publication.Publication{ID: "pub-1", KnowledgeBaseID: "kb-1", PublisherID: "alice", PublisherTenantID: 42, Title: "Guide", Audience: publication.Audience{Type: publication.AudienceOrganization}, AccessMode: publication.AccessSubscriber, Status: publication.StatusPublished, PublishedRevision: 2, CreatedAt: now, PublishedAt: now, UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "r0"}
	store := &subscriptionStoreStub{}
	service, err := NewService(store, publicationReaderStub{pub}, revisionReaderStub{value: 4}, WithClock(func() time.Time { return now }), WithIDGenerator(func() (string, error) { return "sub-1", nil }))
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Subscribe(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 42}, "request-1")
	if err != nil || !first.Changed || first.Item.Updated {
		t.Fatalf("first Subscribe() = %+v, %v", first, err)
	}
	second, err := service.Subscribe(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 42}, "request-2")
	if err != nil || second.Changed {
		t.Fatalf("second Subscribe() = %+v, %v", second, err)
	}
	store.value.LastSeenRevision = 2
	listed, err := service.List(context.Background(), Actor{UserID: "bob", TenantID: 42})
	if err != nil || len(listed) != 1 || !listed[0].Updated {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
	seen, err := service.MarkSeen(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 42}, "request-3")
	if err != nil || !seen.Changed || seen.Item.Updated {
		t.Fatalf("MarkSeen() = %+v, %v", seen, err)
	}
	ended, err := service.Unsubscribe(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 42}, "request-4")
	if err != nil || !ended.Changed || ended.Item.Subscription.Status != StatusUnsubscribed {
		t.Fatalf("Unsubscribe() = %+v, %v", ended, err)
	}
}

func TestSubscribeEnforcesAudienceAndOwner(t *testing.T) {
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	pub := publication.Publication{ID: "pub-1", KnowledgeBaseID: "kb-1", PublisherID: "alice", PublisherTenantID: 42, Title: "Guide", Audience: publication.Audience{Type: publication.AudienceWorkspaceSet, WorkspaceIDs: []uint64{42}}, AccessMode: publication.AccessSubscriber, Status: publication.StatusPublished, PublishedRevision: 1, CreatedAt: now, PublishedAt: now, UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "r0"}
	service, _ := NewService(&subscriptionStoreStub{}, publicationReaderStub{pub}, revisionReaderStub{1})
	if _, err := service.Subscribe(context.Background(), "pub-1", Actor{UserID: "alice", TenantID: 42}, "r1"); err != ErrOwner {
		t.Fatalf("owner error = %v", err)
	}
	if _, err := service.Subscribe(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 99}, "r2"); err != ErrOutsideAudience {
		t.Fatalf("audience error = %v", err)
	}
}

func TestSubscribeRebindsWorkspaceAndReconcilesConcurrentWinner(t *testing.T) {
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	pub := publication.Publication{ID: "pub-1", KnowledgeBaseID: "kb-1", PublisherID: "alice", PublisherTenantID: 42, Title: "Guide", Audience: publication.Audience{Type: publication.AudienceOrganization}, AccessMode: publication.AccessSubscriber, Status: publication.StatusPublished, PublishedRevision: 2, CreatedAt: now, PublishedAt: now, UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "r0"}
	active := Subscription{ID: "sub-1", PublicationID: "pub-1", SubscriberID: "bob", SubscriberTenantID: 7, Status: StatusActive, NotificationEnabled: true, LastSeenRevision: 2, CreatedAt: now, UpdatedAt: now, LastAuditCorrelationID: "old"}
	store := &subscriptionStoreStub{value: active}
	service, _ := NewService(store, publicationReaderStub{pub}, revisionReaderStub{value: 4}, WithClock(func() time.Time { return now }))
	rebound, err := service.Subscribe(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 42}, "request-rebind")
	if err != nil || !rebound.Changed || rebound.Item.Subscription.SubscriberTenantID != 42 {
		t.Fatalf("workspace rebind = %+v, %v", rebound, err)
	}

	winner := rebound.Item.Subscription
	collision := &subscriptionStoreStub{value: winner, createErr: ErrInvalid, missFirst: true}
	service, _ = NewService(collision, publicationReaderStub{pub}, revisionReaderStub{value: 4}, WithClock(func() time.Time { return now }), WithIDGenerator(func() (string, error) { return "loser", nil }))
	result, err := service.Subscribe(context.Background(), "pub-1", Actor{UserID: "bob", TenantID: 42}, "request-race")
	if err != nil || result.Changed || result.Item.Subscription.ID != winner.ID {
		t.Fatalf("concurrent winner reconciliation = %+v, %v", result, err)
	}
}

func TestSubscriptionMutationRequiresBoundWorkspace(t *testing.T) {
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	pub := publication.Publication{ID: "pub-1", KnowledgeBaseID: "kb-1", PublisherID: "alice", PublisherTenantID: 42, Title: "Guide", Audience: publication.Audience{Type: publication.AudienceOrganization}, AccessMode: publication.AccessSubscriber, Status: publication.StatusPublished, PublishedRevision: 2, CreatedAt: now, PublishedAt: now, UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "r0"}
	store := &subscriptionStoreStub{value: Subscription{ID: "sub-1", PublicationID: "pub-1", SubscriberID: "bob", SubscriberTenantID: 42, Status: StatusActive, NotificationEnabled: true, LastSeenRevision: 2, CreatedAt: now, UpdatedAt: now, LastAuditCorrelationID: "old"}}
	service, _ := NewService(store, publicationReaderStub{pub}, revisionReaderStub{value: 4})
	actor := Actor{UserID: "bob", TenantID: 99}
	if _, err := service.MarkSeen(context.Background(), "pub-1", actor, "request-seen"); err != ErrNotFound {
		t.Fatalf("wrong-workspace MarkSeen error = %v, want ErrNotFound", err)
	}
	if _, err := service.Unsubscribe(context.Background(), "pub-1", actor, "request-end"); err != ErrNotFound {
		t.Fatalf("wrong-workspace Unsubscribe error = %v, want ErrNotFound", err)
	}
}
