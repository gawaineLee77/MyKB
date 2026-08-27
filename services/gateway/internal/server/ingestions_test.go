package server

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type ingestionServiceStub struct {
	lastAction string
	filename   string
	content    string
}

func (s *ingestionServiceStub) Upload(_ context.Context, kbID, filename string, _ int64, source io.Reader, _ access.Identity, _ http.Header) (weknora.Knowledge, error) {
	payload, _ := io.ReadAll(source)
	s.lastAction, s.filename, s.content = "upload", filename, string(payload)
	return ingestionFixture(kbID), nil
}
func (s *ingestionServiceStub) List(_ context.Context, kbID string, page, pageSize int, _ access.Identity, _ http.Header) (weknora.KnowledgePage, error) {
	s.lastAction = "list"
	return weknora.KnowledgePage{Items: []weknora.Knowledge{ingestionFixture(kbID)}, Total: 1, Page: page, PageSize: pageSize}, nil
}
func (s *ingestionServiceStub) Get(_ context.Context, kbID, _ string, _ access.Identity, _ http.Header) (weknora.Knowledge, error) {
	s.lastAction = "get"
	return ingestionFixture(kbID), nil
}
func (s *ingestionServiceStub) Retry(_ context.Context, kbID, _ string, _ access.Identity, _ http.Header) (weknora.Knowledge, error) {
	s.lastAction = "retry"
	return ingestionFixture(kbID), nil
}
func (s *ingestionServiceStub) Cancel(_ context.Context, kbID, _ string, _ access.Identity, _ http.Header) (weknora.Knowledge, error) {
	s.lastAction = "cancel"
	return ingestionFixture(kbID), nil
}

func TestProductIngestionLifecycleRoutes(t *testing.T) {
	service := &ingestionServiceStub{}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Ingestions: service})

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "guide.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("synthetic-pdf"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases/kb-rag/ingestions", &upload)
	request.Header.Set("Authorization", "Bearer test")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || service.filename != "guide.pdf" || service.content != "synthetic-pdf" {
		t.Fatalf("upload status=%d service=%+v body=%s", recorder.Code, service, recorder.Body.String())
	}

	for _, testCase := range []struct {
		method string
		path   string
		action string
		status int
	}{
		{http.MethodGet, "/api/v1/knowledge-bases/kb-rag/ingestions?page=1&page_size=20", "list", http.StatusOK},
		{http.MethodGet, "/api/v1/knowledge-bases/kb-rag/ingestions/doc-1", "get", http.StatusOK},
		{http.MethodPost, "/api/v1/knowledge-bases/kb-rag/ingestions/doc-1/retry", "retry", http.StatusAccepted},
		{http.MethodPost, "/api/v1/knowledge-bases/kb-rag/ingestions/doc-1/cancel", "cancel", http.StatusAccepted},
	} {
		request = httptest.NewRequest(testCase.method, testCase.path, nil)
		request.Header.Set("Authorization", "Bearer test")
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != testCase.status || service.lastAction != testCase.action {
			t.Fatalf("%s %s status=%d action=%s body=%s", testCase.method, testCase.path, recorder.Code, service.lastAction, recorder.Body.String())
		}
	}
}

func TestProductIngestionRoutesValidateAuthAndPagination(t *testing.T) {
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Ingestions: &ingestionServiceStub{}})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-rag/ingestions", nil))
	assertErrorCode(t, recorder, http.StatusUnauthorized, "auth.required")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/kb-rag/ingestions?page_size=101", strings.NewReader(""))
	request.Header.Set("Authorization", "Bearer test")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assertErrorCode(t, recorder, http.StatusBadRequest, "ingestion.invalid_request")
}

func ingestionFixture(kbID string) weknora.Knowledge {
	return weknora.Knowledge{ID: "doc-1", KnowledgeBaseID: kbID, Type: "file", FileName: "guide.pdf", FileSize: 13, ParseStatus: "pending"}
}
