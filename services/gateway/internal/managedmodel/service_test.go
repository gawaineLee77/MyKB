package managedmodel

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type upstreamStub struct {
	models          []weknora.Model
	created         weknora.ModelWriteRequest
	updated         weknora.ModelWriteRequest
	credential      string
	deleted         string
	tested          weknora.ModelTestRequest
	testResult      weknora.ModelTestResult
	listErr         error
	credentialError error
}

func (s *upstreamStub) ListModels(context.Context, http.Header) ([]weknora.Model, error) {
	return append([]weknora.Model(nil), s.models...), s.listErr
}
func (s *upstreamStub) CreateModel(_ context.Context, request weknora.ModelWriteRequest, _ http.Header) (weknora.Model, error) {
	s.created = request
	model := weknora.Model{ID: "override-1", Name: request.Name, DisplayName: request.DisplayName, Type: request.Type, Status: "active"}
	s.models = append(s.models, model)
	return model, nil
}
func (s *upstreamStub) UpdateModel(_ context.Context, id string, request weknora.ModelWriteRequest, _ http.Header) (weknora.Model, error) {
	s.updated = request
	return weknora.Model{ID: id, Name: request.Name, DisplayName: request.DisplayName, Type: request.Type, Status: "active"}, nil
}
func (s *upstreamStub) ReplaceModelCredential(_ context.Context, _ string, key string, _ http.Header) error {
	s.credential = key
	return s.credentialError
}
func (s *upstreamStub) DeleteModel(_ context.Context, id string, _ http.Header) error {
	s.deleted = id
	return nil
}
func (s *upstreamStub) TestModel(_ context.Context, request weknora.ModelTestRequest, _ http.Header) (weknora.ModelTestResult, error) {
	s.tested = request
	return s.testResult, nil
}

type auditStub struct{ events []agentaudit.Event }

func (a *auditStub) Record(_ context.Context, event agentaudit.Event) error {
	a.events = append(a.events, event)
	return nil
}

func TestSnapshotIsRedactedAndRequiresExactManagedDefaults(t *testing.T) {
	upstream := &upstreamStub{models: managedDefaults()}
	service := mustService(t, upstream, &auditStub{}, Policy{})

	snapshot, err := service.Snapshot(context.Background(), principal("viewer"), http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Ready || len(snapshot.Defaults) != 3 || len(snapshot.Overrides) != 0 || snapshot.OverridesEnabled {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	for _, descriptor := range snapshot.Defaults {
		if !descriptor.Managed || !descriptor.Default || !descriptor.Available || descriptor.Scope != "organization" {
			t.Fatalf("descriptor = %+v", descriptor)
		}
	}

	upstream.models[1].Status = "inactive"
	snapshot, err = service.Snapshot(context.Background(), principal("viewer"), http.Header{})
	if err != nil || snapshot.Ready || snapshot.Defaults[1].Available {
		t.Fatalf("unhealthy snapshot = %+v, %v", snapshot, err)
	}
}

func TestResolveCreationModelsInjectsManagedDefaultsAndRejectsWrongTypes(t *testing.T) {
	service := mustService(t, &upstreamStub{models: managedDefaults()}, &auditStub{}, Policy{})
	embedding, chat, err := service.ResolveCreationModels(context.Background(), "", "", principal("viewer"), nil)
	if err != nil || embedding != ManagedEmbeddingID || chat != ManagedChatID {
		t.Fatalf("ResolveCreationModels() = %q, %q, %v", embedding, chat, err)
	}
	_, _, err = service.ResolveCreationModels(context.Background(), ManagedChatID, ManagedEmbeddingID, principal("viewer"), nil)
	assertCode(t, err, "models.selection_invalid", http.StatusUnprocessableEntity)
}

func TestOverridesAreCapabilityGatedAndWorkspaceAdminOnly(t *testing.T) {
	input := validInput()
	service := mustService(t, &upstreamStub{models: managedDefaults()}, &auditStub{}, Policy{})
	_, err := service.CreateOverride(context.Background(), input, principal("owner"), requestHeaders())
	assertCode(t, err, "models.overrides_disabled", http.StatusNotFound)

	service = mustService(t, &upstreamStub{models: managedDefaults()}, &auditStub{}, overridePolicy())
	_, err = service.CreateOverride(context.Background(), input, principal("viewer"), requestHeaders())
	assertCode(t, err, "models.override_denied", http.StatusForbidden)
}

func TestOverrideLifecycleKeepsCredentialsWriteOnlyAndAuditsMetadata(t *testing.T) {
	upstream := &upstreamStub{models: managedDefaults(), testResult: weknora.ModelTestResult{Available: true}}
	auditor := &auditStub{}
	service := mustService(t, upstream, auditor, overridePolicy())
	input := validInput()

	descriptor, err := service.CreateOverride(context.Background(), input, principal("owner"), requestHeaders())
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.ID != "override-1" || descriptor.Managed || descriptor.Default || descriptor.Scope != "workspace" {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	if upstream.created.Parameters.APIKey != "top-secret" {
		t.Fatal("credential was not forwarded to the encrypted upstream model store")
	}
	if len(auditor.events) != 1 || auditor.events[0].Operation != "model.override.create" || len(auditor.events[0].KnowledgeBaseIDs) != 0 {
		t.Fatalf("events = %+v", auditor.events)
	}

	input.DisplayName = "Replacement"
	input.APIKey = "rotated-secret"
	descriptor, err = service.UpdateOverride(context.Background(), "override-1", input, principal("admin"), requestHeaders())
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.DisplayName != "Replacement" || upstream.updated.Parameters.APIKey != "" || upstream.credential != "rotated-secret" {
		t.Fatalf("update = %+v, request = %+v", descriptor, upstream.updated)
	}

	result, err := service.TestOverride(context.Background(), OverrideInput{
		Name: "text-embedding-3-small", DisplayName: "Test", Type: "Embedding", Provider: "openai",
		BaseURL: "https://models.internal.example/v1", Dimension: 1536,
	}, "override-1", principal("owner"), requestHeaders())
	if err != nil || !result.Available || upstream.tested.ModelID != "override-1" || upstream.tested.APIKey != "" {
		t.Fatalf("test result = %+v, request = %+v, err = %v", result, upstream.tested, err)
	}

	if err := service.DeleteOverride(context.Background(), "override-1", principal("owner"), requestHeaders()); err != nil {
		t.Fatal(err)
	}
	if upstream.deleted != "override-1" || len(auditor.events) != 4 {
		t.Fatalf("deleted = %q, events = %d", upstream.deleted, len(auditor.events))
	}
}

func TestOverridePolicyRejectsSSRFProviderAndMalformedEmbedding(t *testing.T) {
	service := mustService(t, &upstreamStub{models: managedDefaults()}, &auditStub{}, overridePolicy())
	tests := []struct {
		name  string
		input OverrideInput
		code  string
	}{
		{"http", mutate(validInput(), func(input *OverrideInput) { input.BaseURL = "http://models.internal.example/v1" }), "models.endpoint_denied"},
		{"host", mutate(validInput(), func(input *OverrideInput) { input.BaseURL = "https://127.0.0.1/v1" }), "models.endpoint_denied"},
		{"userinfo", mutate(validInput(), func(input *OverrideInput) { input.BaseURL = "https://user@models.internal.example/v1" }), "models.override_invalid"},
		{"query", mutate(validInput(), func(input *OverrideInput) { input.BaseURL = "https://models.internal.example/v1?token=x" }), "models.override_invalid"},
		{"provider", mutate(validInput(), func(input *OverrideInput) { input.Provider = "unknown" }), "models.provider_denied"},
		{"dimension", mutate(validInput(), func(input *OverrideInput) { input.Dimension = 0 }), "models.override_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.CreateOverride(context.Background(), test.input, principal("owner"), requestHeaders())
			var productError *Error
			if !errors.As(err, &productError) || productError.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestManagedDefaultsCannotBeMutatedThroughOverrideFacade(t *testing.T) {
	service := mustService(t, &upstreamStub{models: managedDefaults()}, &auditStub{}, overridePolicy())
	_, err := service.UpdateOverride(context.Background(), ManagedChatID, validInput(), principal("owner"), requestHeaders())
	assertCode(t, err, "models.override_not_found", http.StatusNotFound)
	if err := service.DeleteOverride(context.Background(), ManagedEmbeddingID, principal("owner"), requestHeaders()); err == nil {
		t.Fatal("managed model deletion succeeded")
	} else {
		assertCode(t, err, "models.override_not_found", http.StatusNotFound)
	}
}

func TestOverrideQuotaAndServicePolicyFailClosed(t *testing.T) {
	if _, err := NewService(&upstreamStub{}, &auditStub{}, Policy{OverridesEnabled: true}); err == nil {
		t.Fatal("enabled override service accepted empty allow-lists")
	}
	models := managedDefaults()
	for index := 0; index < maxOverrides; index++ {
		models = append(models, weknora.Model{ID: "override-" + string(rune('a'+index)), Type: "KnowledgeQA", Status: "active"})
	}
	service := mustService(t, &upstreamStub{models: models}, &auditStub{}, overridePolicy())
	_, err := service.CreateOverride(context.Background(), validInput(), principal("owner"), requestHeaders())
	assertCode(t, err, "models.override_quota", http.StatusUnprocessableEntity)
}

func managedDefaults() []weknora.Model {
	return []weknora.Model{
		{ID: ManagedChatID, Type: "KnowledgeQA", IsBuiltin: true, IsDefault: true, Status: "active"},
		{ID: ManagedEmbeddingID, Type: "Embedding", IsBuiltin: true, IsDefault: true, Status: "active"},
		{ID: ManagedRerankID, Type: "Rerank", IsBuiltin: true, IsDefault: true, Status: "active"},
	}
}

func principal(role string) weknora.Principal {
	return weknora.Principal{
		User: &weknora.User{ID: "alice", TenantID: 42}, Tenant: &weknora.Tenant{ID: 42},
		Memberships: []weknora.Membership{{TenantID: 42, Role: role}},
	}
}

func requestHeaders() http.Header { return http.Header{"X-Request-Id": []string{"request-1"}} }

func overridePolicy() Policy {
	return Policy{OverridesEnabled: true, AllowedProviders: map[string]bool{"generic": true, "openai": true}, AllowedHosts: map[string]bool{"models.internal.example": true}}
}

func validInput() OverrideInput {
	return OverrideInput{Name: "text-embedding-3-small", DisplayName: "Internal embedding", Type: "Embedding", Provider: "openai", BaseURL: "https://models.internal.example/v1", APIKey: "top-secret", Dimension: 1536}
}

func mutate(input OverrideInput, change func(*OverrideInput)) OverrideInput {
	change(&input)
	return input
}

func mustService(t *testing.T, upstream Upstream, auditor agentaudit.Recorder, policy Policy) *Service {
	t.Helper()
	service, err := NewService(upstream, auditor, policy)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func assertCode(t *testing.T, err error, code string, status int) {
	t.Helper()
	var productError *Error
	if !errors.As(err, &productError) || productError.Code != code || productError.StatusCode != status {
		t.Fatalf("error = %v, want %s (%d)", err, code, status)
	}
}
