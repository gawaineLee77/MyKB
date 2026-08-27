package access

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
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

type actionMatcherFunc func(string, string) (authorization.Action, bool)

func (function actionMatcherFunc) Match(method, path string) (authorization.Action, bool) {
	return function(method, path)
}

type decisionStub struct {
	roles map[string]authorization.Role
	errs  map[string]error
}

func (s *decisionStub) Decide(_ context.Context, kbID string, _ authorization.Principal, _ http.Header) (authorization.Decision, error) {
	if err := s.errs[kbID]; err != nil {
		return authorization.Decision{}, err
	}
	return authorization.Decision{KnowledgeBaseID: kbID, Role: s.roles[kbID]}, nil
}

func (s *decisionStub) Authorize(ctx context.Context, kbID string, principal authorization.Principal, action authorization.Action, headers http.Header) (authorization.Decision, error) {
	decision, err := s.Decide(ctx, kbID, principal, headers)
	if err != nil {
		return authorization.Decision{}, err
	}
	if !decision.Allows(action) {
		return decision, authorization.ErrDenied
	}
	return decision, nil
}

type sessionScopeStub struct {
	items map[string][]string
}

type auditRecorderStub struct {
	events []audit.Event
}

func (s *auditRecorderStub) Record(_ context.Context, event audit.Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *sessionScopeStub) ListKnowledgeBases(_ context.Context, sessionID string) ([]string, error) {
	return s.items[sessionID], nil
}

func (s *sessionScopeStub) RecordKnowledgeBases(_ context.Context, sessionID string, kbIDs []string, _ time.Time) error {
	s.items[sessionID] = unique(append(s.items[sessionID], kbIDs...))
	return nil
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

func TestPhase2RoleActionEnforcement(t *testing.T) {
	actions := actionMatcherFunc(func(method, _ string) (authorization.Action, bool) {
		switch method {
		case http.MethodGet:
			return authorization.ActionRead, true
		case http.MethodPost:
			return authorization.ActionEditContent, true
		case http.MethodPut:
			return authorization.ActionConfigure, true
		default:
			return "", false
		}
	})
	tests := []struct {
		name   string
		role   authorization.Role
		method string
		allow  bool
	}{
		{name: "viewer reads", role: authorization.RoleViewer, method: http.MethodGet, allow: true},
		{name: "viewer cannot edit", role: authorization.RoleViewer, method: http.MethodPost},
		{name: "editor edits", role: authorization.RoleEditor, method: http.MethodPost, allow: true},
		{name: "editor cannot configure", role: authorization.RoleEditor, method: http.MethodPut},
		{name: "owner configures", role: authorization.RoleOwner, method: http.MethodPut, allow: true},
		{name: "peer cannot read", role: authorization.RoleNone, method: http.MethodGet},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gate, err := NewPhase2Gate(&fakeProfiles{items: map[string]profile.Profile{}}, &fakeResolver{}, actions, &decisionStub{
				roles: map[string]authorization.Role{"kb-1": test.role}, errs: map[string]error{},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			request := mustRequest(test.method, "http://gateway/api/v1/knowledge-bases/kb-1", "")
			err = gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "user-1", TenantID: 42})
			if test.allow && err != nil {
				t.Fatalf("authorized request failed: %v", err)
			}
			if !test.allow && errorCode(err) != "resource.not_found" {
				t.Fatalf("denied request error = %v", err)
			}
		})
	}
}

func TestPhase2EditorConfigurationIsMetadataOnly(t *testing.T) {
	gate, err := NewPhase2Gate(
		&fakeProfiles{items: map[string]profile.Profile{}}, &fakeResolver{},
		actionMatcherFunc(func(string, string) (authorization.Action, bool) { return authorization.ActionConfigure, true }),
		&decisionStub{roles: map[string]authorization.Role{"kb-1": authorization.RoleEditor}, errs: map[string]error{}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		body  string
		allow bool
	}{
		{name: "name and description", body: `{"name":"Updated","description":"Safe metadata"}`, allow: true},
		{name: "index configuration", body: `{"name":"Updated","config":{"chunking_config":{"chunk_size":1}}}`, allow: false},
		{name: "missing required name", body: `{"description":"Only"}`, allow: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := mustRequest(http.MethodPut, "http://gateway/api/v1/knowledge-bases/kb-1", test.body)
			request.Header.Set("Content-Type", "application/json")
			err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "editor", TenantID: 42})
			if test.allow && err != nil {
				t.Fatalf("metadata update denied: %v", err)
			}
			if !test.allow && errorCode(err) != "resource.not_found" {
				t.Fatalf("unsafe update error = %v", err)
			}
		})
	}
}

func TestPhase2ListFiltersEveryUnauthorizedKB(t *testing.T) {
	gate, err := NewPhase2Gate(
		&fakeProfiles{items: map[string]profile.Profile{}},
		&fakeResolver{},
		actionMatcherFunc(func(string, string) (authorization.Action, bool) { return authorization.ActionDiscover, true }),
		&decisionStub{roles: map[string]authorization.Role{
			"owned": authorization.RoleOwner, "shared": authorization.RoleViewer, "peer": authorization.RoleNone,
		}, errs: map[string]error{}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := mustRequest(http.MethodGet, "http://app/api/v1/knowledge-bases", "")
	request = request.WithContext(WithIdentity(request.Context(), Identity{UserID: "alice", TenantID: 42}))
	response := &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header), Request: request,
		Body: io.NopCloser(strings.NewReader(`{"success":true,"data":[{"id":"owned"},{"id":"shared"},{"id":"peer"}]}`)),
	}
	if err := gate.FilterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if strings.Contains(string(body), "peer") || !strings.Contains(string(body), "owned") || !strings.Contains(string(body), "shared") {
		t.Fatalf("filtered response = %s", body)
	}
}

func TestPhase2SessionScopeIsReauthorized(t *testing.T) {
	scopes := &sessionScopeStub{items: map[string][]string{"session-1": {"kb-1"}}}
	gate, err := NewPhase2Gate(
		&fakeProfiles{items: map[string]profile.Profile{}},
		&fakeResolver{sessions: map[string]bool{"session-1": true}},
		actionMatcherFunc(func(string, string) (authorization.Action, bool) { return authorization.ActionRead, true }),
		&decisionStub{roles: map[string]authorization.Role{"kb-1": authorization.RoleNone}, errs: map[string]error{}},
		scopes,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := mustRequest(http.MethodGet, "http://gateway/api/v1/messages/session-1/load", "")
	if err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "revoked-user", TenantID: 42}); errorCode(err) != "resource.not_found" {
		t.Fatalf("revoked session scope error = %v", err)
	}
}

func TestPhase2DeniedHighValueActionIsAudited(t *testing.T) {
	recorder := &auditRecorderStub{}
	gate, err := NewPhase2Gate(
		&fakeProfiles{items: map[string]profile.Profile{}}, &fakeResolver{},
		actionMatcherFunc(func(string, string) (authorization.Action, bool) { return authorization.ActionDelete, true }),
		&decisionStub{roles: map[string]authorization.Role{"kb-1": authorization.RoleViewer}, errs: map[string]error{}},
		nil, recorder,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := mustRequest(http.MethodDelete, "http://gateway/api/v1/knowledge-bases/kb-1", "")
	request.Header.Set("X-Request-ID", "request-denied")
	if err := gate.AuthorizeRequest(context.Background(), request, Identity{UserID: "bob", TenantID: 42}); errorCode(err) != "resource.not_found" {
		t.Fatalf("delete denial = %v", err)
	}
	if len(recorder.events) != 1 || recorder.events[0].Action != "authorization.denied.delete" || recorder.events[0].Outcome != audit.OutcomeDenied {
		t.Fatalf("audit events = %+v", recorder.events)
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
