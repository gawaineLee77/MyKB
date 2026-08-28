package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/catalog"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type publicationServiceStub struct {
	request publication.WriteRequest
	result  publication.Publication
	err     error
}

func (s *publicationServiceStub) Publish(_ context.Context, _ string, _ publication.Actor, request publication.WriteRequest, _ http.Header) (publication.Publication, error) {
	s.request = request
	return s.result, s.err
}
func (s *publicationServiceStub) Update(_ context.Context, _ string, _ publication.Actor, request publication.WriteRequest, _ http.Header) (publication.Publication, error) {
	s.request = request
	return s.result, s.err
}
func (s *publicationServiceStub) Unpublish(context.Context, string, publication.Actor, int64, string, http.Header) (publication.Publication, error) {
	return s.result, s.err
}
func (s *publicationServiceStub) GetForOwner(context.Context, string, publication.Actor, http.Header) (publication.Publication, error) {
	return s.result, s.err
}

type catalogServiceStub struct{ item catalog.Item }

func (s catalogServiceStub) List(_ context.Context, _ catalog.Principal, filter catalog.Filter) (catalog.Page, error) {
	return catalog.Page{Items: []catalog.Item{s.item}, Total: 1, Page: filter.Page, PageSize: filter.PageSize}, nil
}
func (s catalogServiceStub) Get(context.Context, catalog.Principal, string) (catalog.Item, error) {
	return s.item, nil
}

type subscriptionServiceStub struct{ result subscription.Result }

func (s subscriptionServiceStub) Subscribe(context.Context, string, subscription.Actor, string) (subscription.Result, error) {
	return s.result, nil
}
func (s subscriptionServiceStub) Unsubscribe(context.Context, string, subscription.Actor, string) (subscription.Result, error) {
	return s.result, nil
}
func (s subscriptionServiceStub) MarkSeen(context.Context, string, subscription.Actor, string) (subscription.Result, error) {
	return s.result, nil
}
func (s subscriptionServiceStub) List(context.Context, subscription.Actor) ([]subscription.Item, error) {
	return []subscription.Item{s.result.Item}, nil
}

func TestPublicationCatalogAndSubscriptionEndpoints(t *testing.T) {
	now := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	pub := publication.Publication{ID: "pub-1", KnowledgeBaseID: "kb-1", PublisherID: "alice", PublisherTenantID: 42, Title: "Guide", Audience: publication.Audience{Type: publication.AudienceOrganization}, AccessMode: publication.AccessOrganizationPublic, Status: publication.StatusPublished, PublishedRevision: 1, CreatedAt: now, PublishedAt: now, UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "r0"}
	pubService := &publicationServiceStub{result: pub}
	subResult := subscription.Result{Item: subscription.Item{Subscription: subscription.Subscription{ID: "sub-1", PublicationID: "pub-1", SubscriberID: "bob", SubscriberTenantID: 42, Status: subscription.StatusActive, NotificationEnabled: true, LastSeenRevision: 1, CreatedAt: now, UpdatedAt: now, LastAuditCorrelationID: "r0"}, Publication: pub, CurrentRevision: 1}, Changed: true}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{
		Principals:   principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) { return testPrincipal("bob", 42), nil }),
		Publications: pubService, Catalog: catalogServiceStub{catalog.Item{Publication: pub, CurrentRevision: 1, CanRead: true}}, Subscriptions: subscriptionServiceStub{subResult},
	})
	tests := []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/api/v1/mindcreek/catalog?page=1&page_size=20", "", http.StatusOK},
		{http.MethodGet, "/api/v1/mindcreek/publications/pub-1", "", http.StatusOK},
		{http.MethodGet, "/api/v1/mindcreek/me/subscriptions", "", http.StatusOK},
		{http.MethodPost, "/api/v1/mindcreek/publications/pub-1/subscription", "", http.StatusCreated},
		{http.MethodDelete, "/api/v1/mindcreek/publications/pub-1/subscription", "", http.StatusOK},
		{http.MethodPost, "/api/v1/mindcreek/publications/pub-1/mark-seen", "", http.StatusOK},
		{http.MethodGet, "/api/v1/mindcreek/knowledge-bases/kb-1/publication", "", http.StatusOK},
		{http.MethodPost, "/api/v1/mindcreek/knowledge-bases/kb-1/publication", `{"title":"Guide","audience":{"type":"organization"},"access_mode":"organization_public"}`, http.StatusCreated},
		{http.MethodPatch, "/api/v1/mindcreek/knowledge-bases/kb-1/publication", `{"title":"Guide","audience":{"type":"organization"},"access_mode":"organization_public","expected_row_version":1}`, http.StatusOK},
		{http.MethodDelete, "/api/v1/mindcreek/knowledge-bases/kb-1/publication", `{"expected_row_version":1}`, http.StatusOK},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Authorization", "Bearer valid")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
	}
	if pubService.request.CorrelationID == "" {
		t.Fatal("publication request did not receive correlation ID")
	}
}

func TestPublicationTypedErrors(t *testing.T) {
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) { return testPrincipal("alice", 42), nil }), Publications: &publicationServiceStub{err: publication.ErrPersonalNotes}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mindcreek/knowledge-bases/notes-1/publication", strings.NewReader(`{"title":"No","audience":{"type":"organization"},"access_mode":"subscriber"}`))
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assertErrorCode(t, recorder, http.StatusForbidden, "personal_notes.publication_disabled")
}
