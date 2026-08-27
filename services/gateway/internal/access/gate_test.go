package access

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type fakeProfiles struct {
	items map[string]profile.Profile
}

func (f *fakeProfiles) Get(_ context.Context, id string) (profile.Profile, error) {
	item, ok := f.items[id]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return item, nil
}

func (f *fakeProfiles) ForbiddenPersonalNoteIDs(_ context.Context, userID string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for id, item := range f.items {
		if item.ProductMode == profile.ModePersonalNotes && item.OwnerUserID != userID {
			result[id] = struct{}{}
		}
	}
	return result, nil
}

type fakeResolver struct {
	knowledge map[string]string
	chunks    map[string]string
	agents    map[string]weknora.AgentScope
	sessions  map[string]bool
}

func (f *fakeResolver) KnowledgeBaseForKnowledge(_ context.Context, id string, _ http.Header) (string, error) {
	return f.knowledge[id], nil
}

func (f *fakeResolver) KnowledgeBaseForChunk(_ context.Context, id string, _ http.Header) (string, error) {
	return f.chunks[id], nil
}

func (f *fakeResolver) ValidateSession(_ context.Context, id string, _ http.Header) error {
	if f.sessions == nil || f.sessions[id] {
		return nil
	}
	return &weknora.Error{Code: "upstream.not_found", StatusCode: http.StatusNotFound}
}

func (f *fakeResolver) AgentKnowledgeBases(_ context.Context, id string, _ http.Header) (weknora.AgentScope, error) {
	return f.agents[id], nil
}

func TestDirectAndIndirectNoteAuthorization(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {UpstreamKBID: "alice-note", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly},
	}}
	gate, _ := NewGate(profiles, &fakeResolver{knowledge: map[string]string{"knowledge-1": "alice-note"}, chunks: map[string]string{"chunk-1": "alice-note"}})
	for _, requestPath := range []string{
		"/api/v1/knowledge-bases/alice-note",
		"/api/v1/knowledge-bases/alice-note/faq/entries",
		"/api/v1/knowledge-bases/alice-note/tags",
		"/api/v1/knowledgebase/alice-note/wiki/pages",
		"/api/v1/knowledge/knowledge-1",
		"/api/v1/chunks/knowledge-1",
		"/api/v1/chunks/by-id/chunk-1",
	} {
		request, _ := http.NewRequest(http.MethodGet, "http://gateway"+requestPath, nil)
		if err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "alice", TenantID: 42}); err != nil {
			t.Fatalf("owner denied for %s: %v", requestPath, err)
		}
		if err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "bob", TenantID: 42}); errorCode(err) != "resource.not_found" {
			t.Fatalf("non-owner error for %s = %v", requestPath, err)
		}
	}
}

func TestNoteSharingDeniedForOwner(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {UpstreamKBID: "alice-note", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly},
	}}
	gate, _ := NewGate(profiles, &fakeResolver{})
	request, _ := http.NewRequest(http.MethodPost, "http://gateway/api/v1/knowledge-bases/alice-note/shares", nil)
	err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "alice", TenantID: 42})
	if errorCode(err) != "personal_notes.sharing_disabled" {
		t.Fatalf("share error = %v", err)
	}
}

func TestKBListFiltersAnotherUsersNotes(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes},
	}}
	gate, _ := NewGate(profiles, &fakeResolver{})
	request, _ := http.NewRequest(http.MethodGet, "http://app/api/v1/knowledge-bases", nil)
	request = request.WithContext(WithIdentity(request.Context(), Identity{UserID: "bob", TenantID: 42}))
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":[{"id":"alice-note"},{"id":"rag-1"}]}`)),
		Request:    request,
	}
	if err := gate.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "alice-note") || !strings.Contains(string(body), "rag-1") {
		t.Fatalf("filtered response = %s", body)
	}
}

func TestRetrievalScopesAndSessionsAreGuarded(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {UpstreamKBID: "alice-note", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly},
	}}
	resolver := &fakeResolver{
		knowledge: map[string]string{"knowledge-1": "alice-note"},
		agents:    map[string]weknora.AgentScope{"agent-1": {SelectionMode: "selected", KnowledgeBaseIDs: []string{"alice-note"}}},
		sessions:  map[string]bool{"session-1": true},
	}
	gate, _ := NewGate(profiles, resolver)
	for _, body := range []string{
		`{"query":"secret","knowledge_base_ids":["alice-note"]}`,
		`{"query":"secret","knowledge_ids":["knowledge-1"]}`,
		`{"query":"secret","agent_id":"agent-1"}`,
		`{"config":{"knowledge_bases":["alice-note"]}}`,
	} {
		request, _ := http.NewRequest(http.MethodPost, "http://gateway/api/v1/knowledge-chat/session-1", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "bob", TenantID: 42})
		if errorCode(err) != "resource.not_found" {
			t.Fatalf("body %s error = %v", body, err)
		}
	}
}

func TestSessionGuessFailsClosed(t *testing.T) {
	gate, _ := NewGate(&fakeProfiles{items: map[string]profile.Profile{}}, &fakeResolver{sessions: map[string]bool{"alice-session": false}})
	request, _ := http.NewRequest(http.MethodGet, "http://gateway/api/v1/messages/alice-session/load", nil)
	err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "bob", TenantID: 42})
	if errorCode(err) != "resource.not_found" {
		t.Fatalf("session guess error = %v", err)
	}
}

func TestDerivedContentMatrix(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {UpstreamKBID: "alice-note", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly},
	}}
	resolver := &fakeResolver{
		knowledge: map[string]string{"knowledge-1": "alice-note"},
		chunks:    map[string]string{"chunk-1": "alice-note"},
		sessions:  map[string]bool{"session-1": true},
	}
	gate, _ := NewGate(profiles, resolver)
	for _, requestPath := range []string{
		"/api/v1/knowledge/knowledge-1/preview",
		"/api/v1/knowledge/knowledge-1/download",
		"/api/v1/knowledge/image/knowledge-1/chunk-1",
		"/api/v1/chunks/by-id/chunk-1",
		"/api/v1/knowledge-bases/alice-note/files",
		"/api/v1/knowledgebase/alice-note/wiki/pages",
		"/api/v1/knowledgebase/alice-note/wiki/revisions/page",
		"/api/v1/messages/session-1/load",
		"/api/v1/sessions/session-1/messages/message-1/suggestions",
	} {
		request, _ := http.NewRequest(http.MethodGet, "http://gateway"+requestPath, nil)
		err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "bob", TenantID: 42})
		if errorCode(err) != "resource.not_found" && !strings.Contains(requestPath, "session-1") {
			t.Fatalf("derived path %s error = %v", requestPath, err)
		}
		if strings.Contains(requestPath, "session-1") && err != nil {
			t.Fatalf("owned session validation should pass before upstream response filtering: %v", err)
		}
	}
}

func TestOpaqueLegacyTaskReferencesFailClosed(t *testing.T) {
	gate, _ := NewGate(&fakeProfiles{items: map[string]profile.Profile{}}, &fakeResolver{})
	for _, requestPath := range []string{
		"/api/v1/knowledge/move/progress/task-1",
		"/api/v1/knowledge-bases/copy/progress/task-1",
		"/api/v1/faq/import/progress/task-1",
	} {
		request, _ := http.NewRequest(http.MethodGet, "http://gateway"+requestPath, nil)
		if err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "alice", TenantID: 42}); errorCode(err) != "resource.not_found" {
			t.Fatalf("opaque task path %s error = %v", requestPath, err)
		}
	}
}

func TestSharedListRecomputesTotal(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes},
	}}
	gate, _ := NewGate(profiles, &fakeResolver{})
	request, _ := http.NewRequest(http.MethodGet, "http://app/api/v1/shared-knowledge-bases", nil)
	request = request.WithContext(WithIdentity(request.Context(), Identity{UserID: "bob", TenantID: 42}))
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"success":true,"total":2,"data":[{"knowledge_base_id":"alice-note"},{"knowledge_base_id":"rag-1"}]}`)),
		Request:    request,
	}
	if err := gate.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "alice-note") || !strings.Contains(string(body), `"total":1`) {
		t.Fatalf("filtered shared response = %s", body)
	}
}

func TestOrganizationShareContainerIsFiltered(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes},
	}}
	gate, _ := NewGate(profiles, &fakeResolver{})
	request, _ := http.NewRequest(http.MethodGet, "http://app/api/v1/organizations/org-1/shares", nil)
	request = request.WithContext(WithIdentity(request.Context(), Identity{UserID: "bob", TenantID: 42}))
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{"shares":[{"knowledge_base_id":"alice-note"},{"knowledge_base_id":"rag-1"}],"total":2}}`)),
		Request:    request,
	}
	if err := gate.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "alice-note") || !strings.Contains(string(body), `"total":1`) {
		t.Fatalf("filtered organization shares = %s", body)
	}
}

func TestKBFavoritesAreProtectedAndFiltered(t *testing.T) {
	profiles := &fakeProfiles{items: map[string]profile.Profile{
		"alice-note": {UpstreamKBID: "alice-note", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly},
	}}
	gate, _ := NewGate(profiles, &fakeResolver{})

	for _, request := range []*http.Request{
		mustRequest(http.MethodDelete, "http://gateway/api/v1/user/favorites/kb/alice-note", ""),
		mustRequest(http.MethodPost, "http://gateway/api/v1/user/favorites", `{"type":"kb","id":"alice-note"}`),
	} {
		if request.Body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "bob", TenantID: 42}); errorCode(err) != "resource.not_found" {
			t.Fatalf("favorite request %s error = %v", request.URL.Path, err)
		}
	}

	request, _ := http.NewRequest(http.MethodGet, "http://app/api/v1/user/favorites?type=kb", nil)
	request = request.WithContext(WithIdentity(request.Context(), Identity{UserID: "bob", TenantID: 42}))
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":[{"resource_type":"kb","resource_id":"alice-note"},{"resource_type":"kb","resource_id":"rag-1"}]}`)),
		Request:    request,
	}
	if err := gate.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "alice-note") || !strings.Contains(string(body), "rag-1") {
		t.Fatalf("filtered favorites = %s", body)
	}
}

func mustRequest(method, target, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, target, reader)
	if err != nil {
		panic(err)
	}
	return request
}

func errorCode(err error) string {
	if typed, ok := err.(*Error); ok {
		return typed.Code
	}
	return ""
}
