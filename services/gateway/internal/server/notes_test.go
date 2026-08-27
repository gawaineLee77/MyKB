package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/note"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type noteServiceStub struct {
	lastKBID   string
	lastNoteID string
	lastInput  note.WriteInput
}

func (s *noteServiceStub) List(_ context.Context, kbID string, page, pageSize int, _ access.Identity, _ http.Header) (note.Page, error) {
	s.lastKBID = kbID
	return note.Page{Items: []note.Summary{{ID: "note-1", Title: "Daily"}}, Total: 1, Page: page, PageSize: pageSize}, nil
}
func (s *noteServiceStub) Get(_ context.Context, kbID, noteID string, _ access.Identity, _ http.Header) (note.Note, error) {
	s.lastKBID, s.lastNoteID = kbID, noteID
	return note.Note{ID: noteID, KnowledgeBaseID: kbID, Title: "Daily", Content: "body"}, nil
}
func (s *noteServiceStub) Create(_ context.Context, kbID string, input note.WriteInput, _ access.Identity, _ http.Header) (note.Note, error) {
	s.lastKBID, s.lastInput = kbID, input
	return note.Note{ID: "note-created", KnowledgeBaseID: kbID, Title: input.Title, Content: input.Content}, nil
}
func (s *noteServiceStub) Import(_ context.Context, kbID, filename string, content []byte, _ access.Identity, _ http.Header) (note.Note, error) {
	s.lastKBID = kbID
	s.lastInput = note.WriteInput{Title: filename, Content: string(content)}
	return note.Note{ID: "note-imported", KnowledgeBaseID: kbID, Title: filename, Content: string(content)}, nil
}
func (s *noteServiceStub) Update(_ context.Context, kbID, noteID string, input note.WriteInput, _ access.Identity, _ http.Header) (note.Note, error) {
	s.lastKBID, s.lastNoteID, s.lastInput = kbID, noteID, input
	return note.Note{ID: noteID, KnowledgeBaseID: kbID, Title: input.Title, Content: input.Content}, nil
}
func (s *noteServiceStub) Delete(_ context.Context, kbID, noteID string, _ access.Identity, _ http.Header) error {
	s.lastKBID, s.lastNoteID = kbID, noteID
	return nil
}
func (s *noteServiceStub) ListRevisions(_ context.Context, kbID, noteID string, _ access.Identity) ([]note.Revision, error) {
	s.lastKBID, s.lastNoteID = kbID, noteID
	return []note.Revision{{KnowledgeBaseID: kbID, NoteID: noteID, Version: 2, Title: "Daily"}}, nil
}
func (s *noteServiceStub) GetRevision(_ context.Context, kbID, noteID string, version int, _ access.Identity) (note.Revision, error) {
	s.lastKBID, s.lastNoteID = kbID, noteID
	return note.Revision{KnowledgeBaseID: kbID, NoteID: noteID, Version: version, Title: "Daily", Content: "old"}, nil
}
func (s *noteServiceStub) Restore(_ context.Context, kbID, noteID string, input note.RestoreInput, _ access.Identity, _ http.Header) (note.Note, error) {
	s.lastKBID, s.lastNoteID = kbID, noteID
	return note.Note{ID: noteID, KnowledgeBaseID: kbID, Version: input.ExpectedVersion + 1, Content: "old"}, nil
}

func TestProductNotesRoutes(t *testing.T) {
	service := &noteServiceStub{}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{
		Principals: trustedTestPrincipal,
		Notes:      service,
	})

	for _, testCase := range []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/knowledge-bases/kb-notes/notes?page=1&page_size=10", "", http.StatusOK},
		{http.MethodPost, "/api/v1/knowledge-bases/kb-notes/notes", `{"title":"New","content":"body"}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/knowledge-bases/kb-notes/notes/note-1", "", http.StatusOK},
		{http.MethodPatch, "/api/v1/knowledge-bases/kb-notes/notes/note-1", `{"title":"Edited","content":"body"}`, http.StatusOK},
		{http.MethodDelete, "/api/v1/knowledge-bases/kb-notes/notes/note-1", "", http.StatusOK},
		{http.MethodGet, "/api/v1/knowledge-bases/kb-notes/notes/note-1/revisions", "", http.StatusOK},
		{http.MethodGet, "/api/v1/knowledge-bases/kb-notes/notes/note-1/revisions/1", "", http.StatusOK},
		{http.MethodPost, "/api/v1/knowledge-bases/kb-notes/notes/note-1/restore", `{"expected_version":2,"target_version":1}`, http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Authorization", "Bearer test")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("%s %s status=%d body=%s", testCase.method, testCase.path, recorder.Code, recorder.Body.String())
		}
	}
	if service.lastKBID != "kb-notes" || service.lastNoteID != "note-1" {
		t.Fatalf("route values kb=%q note=%q", service.lastKBID, service.lastNoteID)
	}
}

func TestProductNotesRoutesRequireAuthAndStrictBodies(t *testing.T) {
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Notes: &noteServiceStub{}})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-notes/notes", nil))
	assertErrorCode(t, recorder, http.StatusUnauthorized, "auth.required")

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases/kb-notes/notes", strings.NewReader(`{"title":"New","content":"body","graph":true}`))
	request.Header.Set("Authorization", "Bearer test")
	handler.ServeHTTP(recorder, request)
	assertErrorCode(t, recorder, http.StatusBadRequest, "request.invalid_json")

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-notes/notes?page_size=11", nil)
	request.Header.Set("Authorization", "Bearer test")
	handler.ServeHTTP(recorder, request)
	assertErrorCode(t, recorder, http.StatusBadRequest, "note.invalid_request")
}

func TestProductNotesImportRoute(t *testing.T) {
	service := &noteServiceStub{}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Notes: service})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "daily.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("# Daily\nSynthetic content"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases/kb-notes/notes/import", &body)
	request.Header.Set("Authorization", "Bearer test")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || service.lastInput.Title != "daily.md" {
		t.Fatalf("status=%d input=%+v body=%s", recorder.Code, service.lastInput, recorder.Body.String())
	}
}

var trustedTestPrincipal = principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
	return weknora.Principal{User: &weknora.User{ID: "owner-1"}, Tenant: &weknora.Tenant{ID: 42}}, nil
})
