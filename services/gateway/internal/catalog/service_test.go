package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
)

type publicationStoreStub struct{ items []publication.Publication }

func (s publicationStoreStub) Get(_ context.Context, id string) (publication.Publication, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return publication.Publication{}, publication.ErrNotFound
}
func (s publicationStoreStub) ListPublished(context.Context) ([]publication.Publication, error) {
	return s.items, nil
}

type subscriptionStub struct {
	values map[string]subscription.Subscription
}

func (s subscriptionStub) Effective(_ context.Context, pubID, userID string, tenantID uint64) (subscription.Subscription, error) {
	value, ok := s.values[pubID]
	if !ok {
		return subscription.Subscription{}, subscription.ErrNotFound
	}
	return value, nil
}

type revisionStub struct{ values map[string]int64 }

func (s revisionStub) Current(_ context.Context, kbID string) (int64, error) {
	return s.values[kbID], nil
}

func TestCatalogFiltersAudienceAndDecoratesSubscriptions(t *testing.T) {
	now := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	makePublication := func(id, kb, title string, audience publication.Audience, mode publication.AccessMode) publication.Publication {
		return publication.Publication{ID: id, KnowledgeBaseID: kb, PublisherID: "alice", PublisherTenantID: 42, Title: title, Tags: []string{"policy"}, Audience: audience, AccessMode: mode, Status: publication.StatusPublished, PublishedRevision: 1, CreatedAt: now, PublishedAt: now, UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "r0"}
	}
	service, err := NewService(publicationStoreStub{[]publication.Publication{
		makePublication("pub-org", "kb-org", "Organization Guide", publication.Audience{Type: publication.AudienceOrganization}, publication.AccessOrganizationPublic),
		makePublication("pub-team", "kb-team", "Team Policy", publication.Audience{Type: publication.AudienceWorkspaceSet, WorkspaceIDs: []uint64{42}}, publication.AccessSubscriber),
		makePublication("pub-hidden", "kb-hidden", "Hidden", publication.Audience{Type: publication.AudienceWorkspaceSet, WorkspaceIDs: []uint64{7}}, publication.AccessSubscriber),
	}}, subscriptionStub{map[string]subscription.Subscription{"pub-team": {ID: "sub-1", LastSeenRevision: 1, Status: subscription.StatusActive}}}, revisionStub{map[string]int64{"kb-org": 1, "kb-team": 3}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.List(context.Background(), Principal{UserID: "bob", TenantID: 42}, Filter{Tag: "policy", Page: 1, PageSize: 20})
	if err != nil || page.Total != 2 {
		t.Fatalf("List() = %+v, %v", page, err)
	}
	if !page.Items[0].CanRead || page.Items[0].Subscribed {
		t.Fatalf("organization item = %+v", page.Items[0])
	}
	if !page.Items[1].Subscribed || !page.Items[1].Updated || !page.Items[1].CanRead {
		t.Fatalf("subscriber item = %+v", page.Items[1])
	}
	last, err := service.List(context.Background(), Principal{UserID: "bob", TenantID: 42}, Filter{Tag: "policy", Page: int(^uint(0) >> 1), PageSize: 20})
	if err != nil || last.Total != 2 || len(last.Items) != 0 {
		t.Fatalf("large-page List() = %+v, %v", last, err)
	}
}
