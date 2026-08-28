package publication

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

type publicationStoreStub struct{ value Publication }

func (s *publicationStoreStub) Create(_ context.Context, value Publication) (Publication, error) {
	if s.value.ID != "" {
		return Publication{}, ErrConflict
	}
	s.value = value
	return value, nil
}
func (s *publicationStoreStub) Get(_ context.Context, id string) (Publication, error) {
	if s.value.ID != id {
		return Publication{}, ErrNotFound
	}
	return s.value, nil
}
func (s *publicationStoreStub) GetByKB(_ context.Context, kbID string) (Publication, error) {
	if s.value.KnowledgeBaseID != kbID {
		return Publication{}, ErrNotFound
	}
	return s.value, nil
}
func (s *publicationStoreStub) GetPublishedByKB(ctx context.Context, kbID string) (Publication, error) {
	value, err := s.GetByKB(ctx, kbID)
	if err != nil || !value.Published() {
		return Publication{}, ErrNotFound
	}
	return value, nil
}
func (s *publicationStoreStub) ListPublished(context.Context) ([]Publication, error) {
	return []Publication{s.value}, nil
}
func (s *publicationStoreStub) Update(_ context.Context, value Publication, expected int64) (Publication, error) {
	if s.value.RowVersion != expected {
		return Publication{}, ErrRevisionConflict
	}
	value.RowVersion = expected + 1
	s.value = value
	return value, nil
}
func (s *publicationStoreStub) Unpublish(_ context.Context, id string, expected int64, correlation string, at time.Time) (Publication, error) {
	if s.value.ID != id || s.value.RowVersion != expected {
		return Publication{}, ErrRevisionConflict
	}
	s.value.Status = StatusUnpublished
	s.value.UnpublishedAt = &at
	s.value.UpdatedAt = at
	s.value.RowVersion++
	s.value.LastAuditCorrelationID = correlation
	return s.value, nil
}

type publicationOwnerStub struct{ value ownership.Ownership }

func (s publicationOwnerStub) Resolve(context.Context, string, http.Header) (ownership.Ownership, error) {
	return s.value, nil
}

type publicationRevisionStub struct{ value int64 }

func (s *publicationRevisionStub) Ensure(context.Context, string, string, string, string, string, time.Time) (int64, error) {
	if s.value == 0 {
		s.value = 1
	}
	return s.value, nil
}
func (s *publicationRevisionStub) Current(context.Context, string) (int64, error) {
	return s.value, nil
}

type invalidatorStub struct{ allCalls, audienceCalls int }

func (s *invalidatorStub) InactivatePublication(context.Context, string, string, time.Time) error {
	s.allCalls++
	return nil
}
func (s *invalidatorStub) InactivateOutsideAudience(context.Context, string, Audience, string, time.Time) error {
	s.audienceCalls++
	return nil
}

func TestPublicationLifecycleNormalizesAndRevokesDerivedAccess(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	store := &publicationStoreStub{}
	invalidator := &invalidatorStub{}
	service, err := NewService(store, publicationOwnerStub{ownership.Ownership{
		KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModeRAG,
	}}, &publicationRevisionStub{value: 3}, WithClock(func() time.Time { return now }),
		WithIDGenerator(func() (string, error) { return "pub-1", nil }), WithSubscriptionInvalidator(invalidator))
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Publish(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, WriteRequest{
		Title: "  Team Guide  ", Tags: []string{"Policy", "policy", " RAG "},
		Audience:   Audience{Type: AudienceWorkspaceSet, WorkspaceIDs: []uint64{42, 7}},
		AccessMode: AccessSubscriber, CorrelationID: "request-1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "Team Guide" || len(created.Tags) != 2 || created.Tags[0] != "policy" || created.PublishedRevision != 3 {
		t.Fatalf("created publication = %+v", created)
	}
	_, err = service.Update(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, WriteRequest{
		Title: "Stale", Audience: Audience{Type: AudienceOrganization}, AccessMode: AccessSubscriber,
		ExpectedRowVersion: created.RowVersion + 1, CorrelationID: "request-stale",
	}, nil)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Update() error = %v, want ErrRevisionConflict", err)
	}
	_, err = service.Update(context.Background(), "kb-1", Actor{UserID: "bob", TenantID: 42}, WriteRequest{
		Title: "Peer", Audience: Audience{Type: AudienceOrganization}, AccessMode: AccessSubscriber,
		ExpectedRowVersion: created.RowVersion, CorrelationID: "request-peer",
	}, nil)
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("peer Update() error = %v, want ErrNotOwner", err)
	}
	now = now.Add(time.Minute)
	updated, err := service.Update(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, WriteRequest{
		Title: "Updated", Tags: []string{"rag"}, Audience: Audience{Type: AudienceOrganization},
		AccessMode: AccessOrganizationPublic, ExpectedRowVersion: created.RowVersion, CorrelationID: "request-2",
	}, nil)
	if err != nil || invalidator.audienceCalls != 1 {
		t.Fatalf("Update() = %+v, %v; calls=%d", updated, err, invalidator.audienceCalls)
	}
	now = now.Add(time.Minute)
	ended, err := service.Unpublish(context.Background(), "kb-1", Actor{UserID: "alice", TenantID: 42}, updated.RowVersion, "request-3", nil)
	if err != nil || ended.Published() || invalidator.allCalls != 1 {
		t.Fatalf("Unpublish() = %+v, %v; calls=%d", ended, err, invalidator.allCalls)
	}
}

func TestPublicationRejectsPersonalNotesAndStaleUpdate(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	service, _ := NewService(&publicationStoreStub{}, publicationOwnerStub{ownership.Ownership{
		KnowledgeBaseID: "notes-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModePersonalNotes,
	}}, &publicationRevisionStub{value: 1}, WithClock(func() time.Time { return now }))
	_, err := service.Publish(context.Background(), "notes-1", Actor{UserID: "alice", TenantID: 42}, WriteRequest{
		Title: "No", Audience: Audience{Type: AudienceOrganization}, AccessMode: AccessSubscriber, CorrelationID: "request-1",
	}, nil)
	if !errors.Is(err, ErrPersonalNotes) {
		t.Fatalf("Publish() error = %v", err)
	}
}
