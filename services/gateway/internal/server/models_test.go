package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type managedModelStub struct {
	snapshot      managedmodel.Snapshot
	created       managedmodel.OverrideInput
	resolvedInput [2]string
}

func (s *managedModelStub) Snapshot(context.Context, weknora.Principal, http.Header) (managedmodel.Snapshot, error) {
	return s.snapshot, nil
}
func (s *managedModelStub) ResolveCreationModels(_ context.Context, embedding, chat string, _ weknora.Principal, _ http.Header) (string, string, error) {
	s.resolvedInput = [2]string{embedding, chat}
	return managedmodel.ManagedEmbeddingID, managedmodel.ManagedChatID, nil
}
func (s *managedModelStub) CreateOverride(_ context.Context, input managedmodel.OverrideInput, _ weknora.Principal, _ http.Header) (managedmodel.Descriptor, error) {
	s.created = input
	return managedmodel.Descriptor{ID: "override-1", DisplayName: input.DisplayName, Type: input.Type, Scope: "workspace", Available: true}, nil
}
func (*managedModelStub) UpdateOverride(context.Context, string, managedmodel.OverrideInput, weknora.Principal, http.Header) (managedmodel.Descriptor, error) {
	return managedmodel.Descriptor{}, nil
}
func (*managedModelStub) DeleteOverride(context.Context, string, weknora.Principal, http.Header) error {
	return nil
}
func (*managedModelStub) TestOverride(context.Context, managedmodel.OverrideInput, string, weknora.Principal, http.Header) (managedmodel.TestResult, error) {
	return managedmodel.TestResult{Available: true}, nil
}

func TestManagedModelFacadeReturnsOnlyRedactedContract(t *testing.T) {
	models := &managedModelStub{snapshot: managedmodel.Snapshot{
		Ready:    true,
		Defaults: []managedmodel.Descriptor{{ID: managedmodel.ManagedChatID, DisplayName: "MindCreek Chat", Type: "KnowledgeQA", Managed: true, Default: true, Available: true, Scope: "organization"}},
	}}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Models: models})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/mindcreek/models", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"api_key", "base_url", "provider", "parameters", "credential"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response disclosed %q: %s", forbidden, body)
		}
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, managedmodel.ManagedChatID) {
		t.Fatalf("unexpected response: headers=%v body=%s", recorder.Header(), body)
	}
}

func TestManagedModelOverrideBodyIsStrictAndNeverEchoesCredential(t *testing.T) {
	models := &managedModelStub{}
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Models: models})
	payload := `{"name":"model-a","display_name":"Private chat","type":"KnowledgeQA","provider":"generic","base_url":"https://models.internal.example/v1","api_key":"never-echo"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mindcreek/models/overrides", strings.NewReader(payload))
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || strings.Contains(recorder.Body.String(), "never-echo") || models.created.APIKey != "never-echo" {
		t.Fatalf("status=%d body=%s input=%+v", recorder.Code, recorder.Body.String(), models.created)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/mindcreek/models/overrides", strings.NewReader(`{"name":"model-a","unknown":true}`))
	request.Header.Set("Authorization", "Bearer valid")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assertErrorCode(t, recorder, http.StatusBadRequest, "request.invalid_json")
}

func TestKnowledgeSpaceCreationUsesServerManagedDefaults(t *testing.T) {
	spaces := &fakeKnowledgeSpaces{}
	models := &managedModelStub{}
	principals := principalResolverFunc(func(context.Context, http.Header) (weknora.Principal, error) {
		return testPrincipal("alice", 42), nil
	})
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: principals, Spaces: spaces, Models: models})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-spaces", strings.NewReader(`{"mode":"personal_notes","name":"Zero key notes"}`))
	request.Header.Set("Authorization", "Bearer valid")
	request.Header.Set("Idempotency-Key", "zero-key-create-1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		var response any
		_ = json.Unmarshal(recorder.Body.Bytes(), &response)
		t.Fatalf("status=%d body=%v", recorder.Code, response)
	}
	if models.resolvedInput != [2]string{"", ""} || spaces.createInput.EmbeddingModelID != managedmodel.ManagedEmbeddingID || spaces.createInput.SummaryModelID != managedmodel.ManagedChatID || spaces.createInput.RerankModelID != managedmodel.ManagedRerankID {
		t.Fatalf("resolved=%v create=%+v", models.resolvedInput, spaces.createInput)
	}
}

func TestPhase5DeniesRawModelMutationAndProviderTestRoutes(t *testing.T) {
	handler := NewGateway(testConfig(t), nil, nil, Dependencies{Principals: trustedTestPrincipal, Models: &managedModelStub{}})
	for _, test := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/models"},
		{http.MethodPut, "/api/v1/models/model-1"},
		{http.MethodDelete, "/api/v1/models/model-1"},
		{http.MethodPost, "/api/v1/models/model-1/debug"},
		{http.MethodPut, "/api/v1/models/model-1/credentials"},
		{http.MethodPost, "/api/v1/initialization/remote/check"},
		{http.MethodPost, "/api/v1/initialization/embedding/test"},
		{http.MethodPost, "/api/v1/initialization/rerank/check"},
		{http.MethodPost, "/api/v1/initialization/ollama/models/download"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`)))
		assertErrorCode(t, recorder, http.StatusNotFound, "models.raw_route_disabled")
	}
}
