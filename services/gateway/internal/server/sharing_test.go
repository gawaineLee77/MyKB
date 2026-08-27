package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/grant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type libraryStub struct {
	view library.View
}

func (s *libraryStub) List(_ context.Context, view library.View, page, pageSize int, principal authorization.Principal, _ http.Header) (library.Page, error) {
	s.view = view
	return library.Page{Items: []library.Item{{ID: "kb-1", Name: "Owned", Role: authorization.RoleOwner}}, Total: 1, Page: page, PageSize: pageSize}, nil
}

type grantServiceStub struct {
	create grant.CreateRequest
	update grant.UpdateRequest
	revoke grant.RevokeRequest
	result grant.Grant
	err    error
}

func (s *grantServiceStub) Create(_ context.Context, _ string, _ grant.Actor, request grant.CreateRequest, _ http.Header) (grant.Grant, error) {
	s.create = request
	return s.result, s.err
}
func (s *grantServiceStub) List(context.Context, string, grant.Actor, http.Header) ([]grant.Grant, error) {
	return []grant.Grant{s.result}, s.err
}
func (s *grantServiceStub) Update(_ context.Context, _, _ string, _ grant.Actor, request grant.UpdateRequest, _ http.Header) (grant.Grant, error) {
	s.update = request
	return s.result, s.err
}
func (s *grantServiceStub) Revoke(_ context.Context, _, _ string, _ grant.Actor, request grant.RevokeRequest, _ http.Header) (grant.Grant, error) {
	s.revoke = request
	return s.result, s.err
}

type directoryStub struct{}

func (directoryStub) ListTenantMembers(_ context.Context, tenantID uint64, query string, page, pageSize int, _ http.Header) (weknora.TenantMemberPage, error) {
	return weknora.TenantMemberPage{Items: []weknora.TenantMember{{UserID: "bob", Email: "bob@example.test", Status: "active"}}, Total: 1, Page: page, PageSize: pageSize}, nil
}

type decisionStub struct {
	decision authorization.Decision
	err      error
}

func (s decisionStub) Decide(context.Context, string, authorization.Principal, http.Header) (authorization.Decision, error) {
	return s.decision, s.err
}

func TestSharingProductEndpoints(t *testing.T) {
	libraryService := &libraryStub{}
	grantService := &grantServiceStub{result: grant.Grant{
		ID: "grant-1", KnowledgeBaseID: "kb-1", SubjectType: grant.SubjectUser, SubjectID: "bob",
		Permission: grant.PermissionViewer, Revision: 1,
	}}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{
		Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			return testPrincipal("alice", 42), nil
		}),
		Library: libraryService, Grants: grantService, Directory: directoryStub{},
		Decisions: decisionStub{decision: authorization.Decision{
			KnowledgeBaseID: "kb-1", Role: authorization.RoleEditor, ProductMode: profile.ModeRAG,
		}},
	})

	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodGet, path: "/api/v1/mindcreek/knowledge-bases?view=shared", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/mindcreek/knowledge-bases/kb-1/access", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/mindcreek/users?q=bob", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/mindcreek/knowledge-bases/kb-1/grants", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/mindcreek/knowledge-bases/kb-1/grants", body: `{"subject_type":"user","subject_id":"bob","permission":"viewer"}`, status: http.StatusCreated},
		{method: http.MethodPatch, path: "/api/v1/mindcreek/knowledge-bases/kb-1/grants/grant-1", body: `{"expected_revision":1,"permission":"editor"}`, status: http.StatusOK},
		{method: http.MethodDelete, path: "/api/v1/mindcreek/knowledge-bases/kb-1/grants/grant-1", body: `{"expected_revision":2}`, status: http.StatusOK},
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
	if libraryService.view != library.ViewShared || grantService.create.CorrelationID == "" ||
		grantService.update.ExpectedRevision != 1 || grantService.revoke.ExpectedRevision != 2 {
		t.Fatalf("captured library/grant requests are incomplete: view=%q create=%+v update=%+v revoke=%+v", libraryService.view, grantService.create, grantService.update, grantService.revoke)
	}
}

func TestAccessEndpointHidesNoRole(t *testing.T) {
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{
		Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
			return testPrincipal("peer", 42), nil
		}),
		Decisions: decisionStub{decision: authorization.Decision{KnowledgeBaseID: "kb-private", Role: authorization.RoleNone}},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/mindcreek/knowledge-bases/kb-private/access", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assertErrorCode(t, recorder, http.StatusNotFound, "resource.not_found")
}

func TestSharingAPITypedSafetyErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "personal notes", err: grant.ErrPersonalNotes, status: http.StatusForbidden, code: "personal_notes.sharing_disabled"},
		{name: "non owner", err: grant.ErrNotOwner, status: http.StatusNotFound, code: "resource.not_found"},
		{name: "stale revision", err: grant.ErrRevisionConflict, status: http.StatusConflict, code: "grant.revision_conflict"},
		{name: "unsupported subject", err: grant.ErrSubjectUnsupported, status: http.StatusBadRequest, code: "grant.subject_unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &grantServiceStub{err: test.err}
			handler := NewGateway(testConfig(t), nil, nil, Dependencies{
				Principals: principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) { return testPrincipal("alice", 42), nil }),
				Grants:     service,
			})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/mindcreek/knowledge-bases/kb-1/grants", strings.NewReader(`{"subject_type":"user","subject_id":"bob","permission":"viewer"}`))
			request.Header.Set("Authorization", "Bearer valid")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assertErrorCode(t, recorder, test.status, test.code)
		})
	}
}
