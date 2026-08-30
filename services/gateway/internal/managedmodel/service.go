// Package managedmodel exposes MindCreek's safe managed-model contract while
// keeping provider endpoints and credentials inside the private WeKnora service.
package managedmodel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agentaudit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const (
	ManagedChatID      = "builtin-mindcreek-chat"
	ManagedEmbeddingID = "builtin-mindcreek-embedding"
	ManagedRerankID    = "builtin-mindcreek-rerank"
	maxOverrides       = 12
)

var ErrInvalid = errors.New("invalid managed model request")

type Error struct {
	Code       string
	Message    string
	StatusCode int
	Err        error
}

func (e *Error) Error() string { return e.Code }
func (e *Error) Unwrap() error { return e.Err }

type Upstream interface {
	ListModels(context.Context, http.Header) ([]weknora.Model, error)
	CreateModel(context.Context, weknora.ModelWriteRequest, http.Header) (weknora.Model, error)
	UpdateModel(context.Context, string, weknora.ModelWriteRequest, http.Header) (weknora.Model, error)
	ReplaceModelCredential(context.Context, string, string, http.Header) error
	DeleteModel(context.Context, string, http.Header) error
	TestModel(context.Context, weknora.ModelTestRequest, http.Header) (weknora.ModelTestResult, error)
}

type Policy struct {
	OverridesEnabled bool
	AllowedProviders map[string]bool
	AllowedHosts     map[string]bool
	AllowHTTP        bool
}

type Descriptor struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Managed     bool   `json:"managed"`
	Default     bool   `json:"default"`
	Available   bool   `json:"available"`
	Scope       string `json:"scope"`
}

type Snapshot struct {
	Ready            bool         `json:"ready"`
	Defaults         []Descriptor `json:"defaults"`
	Overrides        []Descriptor `json:"overrides"`
	OverridesEnabled bool         `json:"overrides_enabled"`
}

type DefaultIDs struct {
	Chat      string `json:"chat"`
	Embedding string `json:"embedding"`
	Rerank    string `json:"rerank"`
}

type OverrideInput struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	APIKey      string `json:"api_key,omitempty"`
	Dimension   int    `json:"dimension,omitempty"`
}

type TestResult struct {
	Available bool `json:"available"`
	Dimension int  `json:"dimension,omitempty"`
}

type Service struct {
	upstream Upstream
	auditor  agentaudit.Recorder
	policy   Policy
}

func NewService(upstream Upstream, auditor agentaudit.Recorder, policy Policy) (*Service, error) {
	if upstream == nil || auditor == nil {
		return nil, fmt.Errorf("managed model upstream and auditor are required")
	}
	if policy.OverridesEnabled && (len(policy.AllowedProviders) == 0 || len(policy.AllowedHosts) == 0) {
		return nil, fmt.Errorf("model overrides require non-empty provider and host allow-lists")
	}
	return &Service{upstream: upstream, auditor: auditor, policy: policy}, nil
}

func (s *Service) Snapshot(ctx context.Context, principal weknora.Principal, headers http.Header) (Snapshot, error) {
	models, err := s.upstream.ListModels(ctx, headers)
	if err != nil {
		return Snapshot{}, unavailable(err)
	}
	byID := make(map[string]weknora.Model, len(models))
	for _, model := range models {
		byID[model.ID] = model
	}
	defaults := []Descriptor{
		managedDescriptor(byID[ManagedChatID], ManagedChatID, "MindCreek Chat", "KnowledgeQA"),
		managedDescriptor(byID[ManagedEmbeddingID], ManagedEmbeddingID, "MindCreek Embedding", "Embedding"),
		managedDescriptor(byID[ManagedRerankID], ManagedRerankID, "MindCreek Rerank", "Rerank"),
	}
	ready := true
	for _, descriptor := range defaults {
		ready = ready && descriptor.Available
	}
	overrides := make([]Descriptor, 0)
	if s.policy.OverridesEnabled {
		for _, model := range models {
			if model.IsBuiltin || !supportedType(model.Type) {
				continue
			}
			overrides = append(overrides, Descriptor{
				ID: model.ID, DisplayName: safeDisplayName(model), Type: model.Type,
				Managed: false, Default: false, Available: model.Status == "active", Scope: "workspace",
			})
		}
		sort.Slice(overrides, func(i, j int) bool {
			if overrides[i].Type != overrides[j].Type {
				return overrides[i].Type < overrides[j].Type
			}
			return overrides[i].DisplayName < overrides[j].DisplayName
		})
	}
	return Snapshot{Ready: ready, Defaults: defaults, Overrides: overrides, OverridesEnabled: s.policy.OverridesEnabled}, nil
}

func (s *Service) Defaults(ctx context.Context, principal weknora.Principal, headers http.Header) (DefaultIDs, error) {
	snapshot, err := s.Snapshot(ctx, principal, headers)
	if err != nil {
		return DefaultIDs{}, err
	}
	if !snapshot.Ready {
		return DefaultIDs{}, &Error{Code: "models.defaults_unavailable", Message: "Managed models are not ready", StatusCode: http.StatusServiceUnavailable}
	}
	return DefaultIDs{Chat: ManagedChatID, Embedding: ManagedEmbeddingID, Rerank: ManagedRerankID}, nil
}

// ResolveCreationModels injects defaults and permits only active workspace
// overrides when the override capability is enabled.
func (s *Service) ResolveCreationModels(ctx context.Context, embeddingID, chatID string, principal weknora.Principal, headers http.Header) (string, string, error) {
	snapshot, err := s.Snapshot(ctx, principal, headers)
	if err != nil {
		return "", "", err
	}
	if !snapshot.Ready {
		return "", "", &Error{Code: "models.defaults_unavailable", Message: "Managed models are not ready", StatusCode: http.StatusServiceUnavailable}
	}
	if strings.TrimSpace(embeddingID) == "" {
		embeddingID = ManagedEmbeddingID
	}
	if strings.TrimSpace(chatID) == "" {
		chatID = ManagedChatID
	}
	available := map[string]string{ManagedEmbeddingID: "Embedding", ManagedChatID: "KnowledgeQA"}
	if snapshot.OverridesEnabled {
		for _, model := range snapshot.Overrides {
			if model.Available {
				available[model.ID] = model.Type
			}
		}
	}
	if available[embeddingID] != "Embedding" || available[chatID] != "KnowledgeQA" {
		return "", "", &Error{Code: "models.selection_invalid", Message: "Selected model is unavailable", StatusCode: http.StatusUnprocessableEntity, Err: ErrInvalid}
	}
	return embeddingID, chatID, nil
}

func (s *Service) CreateOverride(ctx context.Context, input OverrideInput, principal weknora.Principal, headers http.Header) (Descriptor, error) {
	if err := s.requireManager(principal); err != nil {
		return Descriptor{}, err
	}
	if err := s.validateInput(input, true); err != nil {
		return Descriptor{}, err
	}
	models, err := s.upstream.ListModels(ctx, headers)
	if err != nil {
		return Descriptor{}, unavailable(err)
	}
	count := 0
	for _, model := range models {
		if !model.IsBuiltin {
			count++
		}
	}
	if count >= maxOverrides {
		return Descriptor{}, &Error{Code: "models.override_quota", Message: "Workspace model override quota reached", StatusCode: http.StatusUnprocessableEntity}
	}
	started := time.Now()
	created, err := s.upstream.CreateModel(ctx, writeRequest(input), headers)
	if err != nil {
		s.record(ctx, principal, headers, "model.override.create", agentaudit.OutcomeFailure, "models.override_failed", time.Since(started))
		return Descriptor{}, translate(err)
	}
	if err := s.record(ctx, principal, headers, "model.override.create", agentaudit.OutcomeSuccess, "", time.Since(started)); err != nil {
		return Descriptor{}, &Error{Code: "audit.unavailable", Message: "Model audit is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	return overrideDescriptor(created), nil
}

func (s *Service) UpdateOverride(ctx context.Context, id string, input OverrideInput, principal weknora.Principal, headers http.Header) (Descriptor, error) {
	if err := s.requireManager(principal); err != nil {
		return Descriptor{}, err
	}
	if err := s.ensureOverride(ctx, id, headers); err != nil {
		return Descriptor{}, err
	}
	if err := s.validateInput(input, false); err != nil {
		return Descriptor{}, err
	}
	request := writeRequest(input)
	request.Parameters.APIKey = ""
	started := time.Now()
	updated, err := s.upstream.UpdateModel(ctx, id, request, headers)
	if err == nil && input.APIKey != "" {
		err = s.upstream.ReplaceModelCredential(ctx, id, input.APIKey, headers)
	}
	if err != nil {
		s.record(ctx, principal, headers, "model.override.update", agentaudit.OutcomeFailure, "models.override_failed", time.Since(started))
		return Descriptor{}, translate(err)
	}
	if err := s.record(ctx, principal, headers, "model.override.update", agentaudit.OutcomeSuccess, "", time.Since(started)); err != nil {
		return Descriptor{}, &Error{Code: "audit.unavailable", Message: "Model audit is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	return overrideDescriptor(updated), nil
}

func (s *Service) DeleteOverride(ctx context.Context, id string, principal weknora.Principal, headers http.Header) error {
	if err := s.requireManager(principal); err != nil {
		return err
	}
	if err := s.ensureOverride(ctx, id, headers); err != nil {
		return err
	}
	started := time.Now()
	if err := s.upstream.DeleteModel(ctx, id, headers); err != nil {
		s.record(ctx, principal, headers, "model.override.delete", agentaudit.OutcomeFailure, "models.override_failed", time.Since(started))
		return translate(err)
	}
	if err := s.record(ctx, principal, headers, "model.override.delete", agentaudit.OutcomeSuccess, "", time.Since(started)); err != nil {
		return &Error{Code: "audit.unavailable", Message: "Model audit is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	return nil
}

func (s *Service) TestOverride(ctx context.Context, input OverrideInput, modelID string, principal weknora.Principal, headers http.Header) (TestResult, error) {
	if err := s.requireManager(principal); err != nil {
		return TestResult{}, err
	}
	requireKey := strings.TrimSpace(modelID) == ""
	if modelID != "" {
		if err := s.ensureOverride(ctx, modelID, headers); err != nil {
			return TestResult{}, err
		}
	}
	if err := s.validateInput(input, requireKey); err != nil {
		return TestResult{}, err
	}
	started := time.Now()
	result, err := s.upstream.TestModel(ctx, weknora.ModelTestRequest{
		Type: input.Type, ModelID: modelID, ModelName: input.Name, BaseURL: input.BaseURL,
		APIKey: input.APIKey, Provider: input.Provider, Source: "remote", Dimension: input.Dimension,
	}, headers)
	if err != nil || !result.Available {
		s.record(ctx, principal, headers, "model.override.test", agentaudit.OutcomeFailure, "models.test_failed", time.Since(started))
		if err != nil {
			return TestResult{}, translate(err)
		}
		return TestResult{Available: false, Dimension: result.Dimension}, nil
	}
	if err := s.record(ctx, principal, headers, "model.override.test", agentaudit.OutcomeSuccess, "", time.Since(started)); err != nil {
		return TestResult{}, &Error{Code: "audit.unavailable", Message: "Model audit is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	return TestResult{Available: true, Dimension: result.Dimension}, nil
}

func (s *Service) ensureOverride(ctx context.Context, id string, headers http.Header) error {
	if strings.TrimSpace(id) == "" || len(id) > 64 {
		return &Error{Code: "models.override_not_found", Message: "Model override not found", StatusCode: http.StatusNotFound}
	}
	models, err := s.upstream.ListModels(ctx, headers)
	if err != nil {
		return unavailable(err)
	}
	for _, model := range models {
		if model.ID == id && !model.IsBuiltin {
			return nil
		}
	}
	return &Error{Code: "models.override_not_found", Message: "Model override not found", StatusCode: http.StatusNotFound}
}

func (s *Service) requireManager(principal weknora.Principal) error {
	if !s.policy.OverridesEnabled {
		return &Error{Code: "models.overrides_disabled", Message: "Model overrides are disabled", StatusCode: http.StatusNotFound}
	}
	if principal.User == nil || principal.Tenant == nil {
		return &Error{Code: "auth.principal_invalid", Message: "Authenticated principal is invalid", StatusCode: http.StatusUnauthorized}
	}
	if principal.User.CanAccessAllTenants {
		return nil
	}
	for _, membership := range principal.Memberships {
		if membership.TenantID == principal.Tenant.ID && (membership.Role == "owner" || membership.Role == "admin") {
			return nil
		}
	}
	return &Error{Code: "models.override_denied", Message: "Workspace model administration is required", StatusCode: http.StatusForbidden}
}

func (s *Service) validateInput(input OverrideInput, requireKey bool) error {
	input.Name = strings.TrimSpace(input.Name)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	if input.Name == "" || len([]rune(input.Name)) > 128 || len([]rune(input.DisplayName)) > 128 || !supportedType(input.Type) {
		return invalid("Model override fields are invalid")
	}
	if !s.policy.AllowedProviders[input.Provider] {
		return &Error{Code: "models.provider_denied", Message: "Model provider is not approved", StatusCode: http.StatusUnprocessableEntity}
	}
	parsed, err := url.Parse(input.BaseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return invalid("Model endpoint is invalid")
	}
	if parsed.Scheme == "http" && !s.policy.AllowHTTP {
		return &Error{Code: "models.endpoint_denied", Message: "Model endpoint must use HTTPS", StatusCode: http.StatusUnprocessableEntity}
	}
	if !s.policy.AllowedHosts[strings.ToLower(parsed.Hostname())] {
		return &Error{Code: "models.endpoint_denied", Message: "Model endpoint host is not approved", StatusCode: http.StatusUnprocessableEntity}
	}
	if requireKey && strings.TrimSpace(input.APIKey) == "" {
		return invalid("Model credential is required")
	}
	if len(input.APIKey) > 4096 {
		return invalid("Model credential is invalid")
	}
	if input.Type == "Embedding" && (input.Dimension < 1 || input.Dimension > 65536) {
		return invalid("Embedding dimension is invalid")
	}
	return nil
}

func managedDescriptor(model weknora.Model, id, name, modelType string) Descriptor {
	available := model.ID == id && model.Type == modelType && model.IsBuiltin && model.IsDefault && model.Status == "active"
	return Descriptor{ID: id, DisplayName: name, Type: modelType, Managed: true, Default: true, Available: available, Scope: "organization"}
}

func overrideDescriptor(model weknora.Model) Descriptor {
	return Descriptor{ID: model.ID, DisplayName: safeDisplayName(model), Type: model.Type, Available: model.Status == "active", Scope: "workspace"}
}

func safeDisplayName(model weknora.Model) string {
	if value := strings.TrimSpace(model.DisplayName); value != "" {
		return value
	}
	return strings.TrimSpace(model.Name)
}

func supportedType(value string) bool {
	return value == "KnowledgeQA" || value == "Embedding" || value == "Rerank"
}

func writeRequest(input OverrideInput) weknora.ModelWriteRequest {
	request := weknora.ModelWriteRequest{
		Name: strings.TrimSpace(input.Name), DisplayName: strings.TrimSpace(input.DisplayName),
		Type: input.Type, Source: "remote", Description: "MindCreek workspace model override",
		Parameters: weknora.ModelWriteParameters{
			BaseURL: strings.TrimSpace(input.BaseURL), APIKey: input.APIKey,
			Provider: strings.ToLower(strings.TrimSpace(input.Provider)),
		},
	}
	if input.Type == "Embedding" {
		request.Parameters.EmbeddingParameters = &weknora.ModelEmbeddingWriteParameters{Dimension: input.Dimension}
	}
	return request
}

func (s *Service) record(ctx context.Context, principal weknora.Principal, headers http.Header, operation string, outcome agentaudit.Outcome, code string, duration time.Duration) error {
	if principal.User == nil || principal.Tenant == nil {
		return agentaudit.ErrInvalid
	}
	correlation := strings.TrimSpace(headers.Get("X-Request-ID"))
	if correlation == "" {
		return agentaudit.ErrInvalid
	}
	return s.auditor.Record(ctx, agentaudit.Event{
		TenantID: principal.Tenant.ID, ActorUserID: principal.User.ID, ClientKind: agentaudit.ClientWeb,
		Operation: operation, KnowledgeBaseIDs: []string{}, Outcome: outcome, ErrorCode: code,
		CorrelationID: correlation, Duration: duration, CreatedAt: time.Now().UTC(),
	})
}

func invalid(message string) error {
	return &Error{Code: "models.override_invalid", Message: message, StatusCode: http.StatusBadRequest, Err: ErrInvalid}
}

func unavailable(err error) error {
	return &Error{Code: "models.unavailable", Message: "Model service is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
}

func translate(err error) error {
	var upstream *weknora.Error
	if errors.As(err, &upstream) {
		switch upstream.Code {
		case "upstream.forbidden":
			return &Error{Code: "models.override_denied", Message: "Workspace model administration is required", StatusCode: http.StatusForbidden, Err: err}
		case "upstream.not_found":
			return &Error{Code: "models.override_not_found", Message: "Model override not found", StatusCode: http.StatusNotFound, Err: err}
		case "upstream.rejected":
			return &Error{Code: "models.override_invalid", Message: "Model provider configuration was rejected", StatusCode: http.StatusUnprocessableEntity, Err: err}
		}
	}
	return unavailable(err)
}
