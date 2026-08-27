// Package space implements product-mode knowledge-base creation and reconciliation.
package space

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/notespolicy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/preset"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type Upstream interface {
	GetKnowledgeBase(context.Context, string, http.Header) (weknora.KnowledgeBase, error)
	CreateKnowledgeBase(context.Context, weknora.CreateKnowledgeBaseRequest, http.Header) (weknora.KnowledgeBase, error)
}

type ProfileStore interface {
	Create(context.Context, profile.Profile) (profile.Profile, error)
	Get(context.Context, string) (profile.Profile, error)
}

type Service struct {
	requests   RequestStore
	profiles   ProfileStore
	upstream   Upstream
	authorizer Authorizer
}

type Authorizer interface {
	Authorize(context.Context, string, authorization.Principal, authorization.Action, http.Header) (authorization.Decision, error)
}

type CreateInput struct {
	Mode             string `json:"mode"`
	IndexProfile     string `json:"index_profile,omitempty"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	EmbeddingModelID string `json:"embedding_model_id"`
	SummaryModelID   string `json:"summary_model_id,omitempty"`
	StorageProvider  string `json:"storage_provider,omitempty"`
}

type CreateResult struct {
	KnowledgeBaseID string               `json:"knowledge_base_id"`
	Name            string               `json:"name"`
	ProductMode     profile.ProductMode  `json:"product_mode"`
	IndexProfile    string               `json:"index_profile"`
	AccessPolicy    profile.AccessPolicy `json:"access_policy"`
	Created         bool                 `json:"created"`
	Reconciled      bool                 `json:"reconciled"`
}

type Error struct {
	Code       string
	Message    string
	StatusCode int
	Err        error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func NewService(requests RequestStore, profiles ProfileStore, upstream Upstream, authorizers ...Authorizer) (*Service, error) {
	if requests == nil || profiles == nil || upstream == nil {
		return nil, fmt.Errorf("creation request store, profile store, and upstream adapter are required")
	}
	service := &Service{requests: requests, profiles: profiles, upstream: upstream}
	if len(authorizers) > 1 {
		return nil, fmt.Errorf("at most one knowledge-space authorizer is supported")
	}
	if len(authorizers) == 1 {
		service.authorizer = authorizers[0]
	}
	return service, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput, idempotencyKey string, identity access.Identity, headers http.Header) (CreateResult, error) {
	normalized, productMode, indexProfile, accessPolicy, err := normalizeCreateInput(input)
	if err != nil {
		return CreateResult{}, err
	}
	definition, err := preset.Build(productMode, normalized.EmbeddingModelID, normalized.SummaryModelID)
	if err != nil || definition.Config.ProfileID != indexProfile || definition.AccessPolicy != accessPolicy {
		return CreateResult{}, &Error{Code: "space.preset_invalid", Message: "Approved knowledge-space preset is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	effectiveConfig, err := definition.JSON()
	if err != nil {
		return CreateResult{}, &Error{Code: "space.preset_invalid", Message: "Approved knowledge-space preset is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return CreateResult{}, err
	}
	if identity.UserID == "" || identity.TenantID == 0 {
		return CreateResult{}, &Error{Code: "auth.principal_invalid", Message: "Authenticated principal is invalid", StatusCode: http.StatusBadGateway}
	}
	requestHash, err := hashCreateInput(normalized)
	if err != nil {
		return CreateResult{}, err
	}
	assignedID, err := newUUID()
	if err != nil {
		return CreateResult{}, &Error{Code: "space.id_generation_failed", Message: "Unable to allocate a knowledge space", StatusCode: http.StatusInternalServerError, Err: err}
	}
	ledger, inserted, err := s.requests.Claim(ctx, CreationRequest{
		TenantID: identity.TenantID, OwnerUserID: identity.UserID, IdempotencyKey: idempotencyKey,
		RequestHash: requestHash, UpstreamKBID: assignedID, ProductMode: string(productMode), IndexProfile: indexProfile,
	})
	if errors.Is(err, ErrIdempotencyConflict) {
		return CreateResult{}, &Error{Code: "request.idempotency_conflict", Message: "Idempotency-Key was already used for a different request", StatusCode: http.StatusConflict, Err: err}
	}
	if err != nil {
		return CreateResult{}, &Error{Code: "space.state_unavailable", Message: "Knowledge-space creation state is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}

	kb, found, err := s.findUpstream(ctx, ledger.UpstreamKBID, headers)
	if err != nil {
		s.recordFailure(ctx, ledger, err)
		return CreateResult{}, err
	}
	created := false
	if !found {
		kb, err = s.upstream.CreateKnowledgeBase(ctx, definition.UpstreamRequest(ledger.UpstreamKBID, normalized.Name, normalized.Description), headers)
		if err != nil {
			// A concurrent request or a lost response may have created the
			// deterministic ID. Resolve it once before recording failure.
			kb, found, _ = s.findUpstream(ctx, ledger.UpstreamKBID, headers)
			if !found {
				translated := translateUpstreamError(err)
				s.recordFailure(ctx, ledger, translated)
				return CreateResult{}, translated
			}
		} else {
			created = true
		}
	}
	if err := verifyUpstreamOwnership(kb, ledger, normalized); err != nil {
		s.recordFailure(ctx, ledger, err)
		return CreateResult{}, err
	}
	expectedProfile := profile.Profile{
		UpstreamKBID: ledger.UpstreamKBID, TenantID: identity.TenantID, OwnerUserID: identity.UserID,
		ProductMode: productMode, SchemaVersion: 1, AccessPolicy: accessPolicy,
		IndexProfile: indexProfile, IndexProfileVersion: definition.Config.ProfileVersion, EffectiveConfig: effectiveConfig,
	}
	if err := s.ensureProfile(ctx, expectedProfile); err != nil {
		s.recordFailure(ctx, ledger, err)
		return CreateResult{}, err
	}
	if err := s.requests.Complete(ctx, ledger); err != nil {
		return CreateResult{}, &Error{Code: "space.state_unavailable", Message: "Knowledge space was created but completion state could not be recorded; retry with the same Idempotency-Key", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	return CreateResult{
		KnowledgeBaseID: ledger.UpstreamKBID, Name: kb.Name, ProductMode: productMode,
		IndexProfile: indexProfile, AccessPolicy: accessPolicy, Created: inserted && created,
		Reconciled: !inserted || !created,
	}, nil
}

func (s *Service) GetProfile(ctx context.Context, kbID string, identity access.Identity, headers http.Header) (profile.Profile, error) {
	if s.authorizer != nil {
		if _, err := s.authorizer.Authorize(ctx, kbID, authorization.Principal{UserID: identity.UserID, TenantID: identity.TenantID}, authorization.ActionRead, headers); err != nil {
			if errors.Is(err, authorization.ErrDenied) || errors.Is(err, authorization.ErrNotFound) {
				return profile.Profile{}, &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound, Err: err}
			}
			return profile.Profile{}, &Error{Code: "space.state_unavailable", Message: "Knowledge-space authorization is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
		}
	} else if _, err := s.upstream.GetKnowledgeBase(ctx, kbID, headers); err != nil {
		return profile.Profile{}, translateUpstreamError(err)
	}
	result, err := s.profiles.Get(ctx, kbID)
	if errors.Is(err, profile.ErrNotFound) {
		return profile.Profile{}, &Error{Code: "profile.not_found", Message: "Product profile not found", StatusCode: http.StatusNotFound}
	}
	if err != nil {
		return profile.Profile{}, &Error{Code: "space.state_unavailable", Message: "Knowledge-space profile is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	if s.authorizer == nil {
		if err := notespolicy.Authorize(result, notespolicy.Principal{UserID: identity.UserID, TenantID: identity.TenantID}, notespolicy.Read); err != nil {
			var policyError *notespolicy.Error
			if errors.As(err, &policyError) {
				return profile.Profile{}, &Error{Code: policyError.Code, Message: policyError.Message, StatusCode: policyError.StatusCode, Err: err}
			}
			return profile.Profile{}, err
		}
	}
	return result, nil
}

func normalizeCreateInput(input CreateInput) (CreateInput, profile.ProductMode, string, profile.AccessPolicy, error) {
	input.Mode = strings.TrimSpace(input.Mode)
	input.IndexProfile = strings.TrimSpace(input.IndexProfile)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.EmbeddingModelID = strings.TrimSpace(input.EmbeddingModelID)
	input.SummaryModelID = strings.TrimSpace(input.SummaryModelID)
	input.StorageProvider = strings.ToLower(strings.TrimSpace(input.StorageProvider))
	if input.StorageProvider == "" {
		input.StorageProvider = "local"
	}
	if input.Name == "" || len([]rune(input.Name)) > 120 || len([]rune(input.Description)) > 1000 || input.EmbeddingModelID == "" {
		return input, "", "", "", &Error{Code: "space.invalid_request", Message: "Name, description, or embedding model is invalid", StatusCode: http.StatusBadRequest}
	}
	if input.StorageProvider != "local" {
		return input, "", "", "", &Error{Code: "space.storage_profile_disabled", Message: "Only the approved local storage profile is enabled in Phase 1", StatusCode: http.StatusBadRequest}
	}
	switch input.Mode {
	case string(profile.ModePersonalNotes):
		if input.IndexProfile == "" {
			input.IndexProfile = "notes_plain"
		}
		if input.IndexProfile != "notes_plain" {
			return input, "", "", "", disabledModeError()
		}
		return input, profile.ModePersonalNotes, input.IndexProfile, profile.PolicyOwnerOnly, nil
	case string(profile.ModeRAG):
		if input.IndexProfile == "" {
			input.IndexProfile = "plain"
		}
		if input.IndexProfile != "plain" {
			return input, "", "", "", disabledModeError()
		}
		return input, profile.ModeRAG, input.IndexProfile, profile.PolicyUpstream, nil
	default:
		return input, "", "", "", disabledModeError()
	}
}

func (s *Service) findUpstream(ctx context.Context, id string, headers http.Header) (weknora.KnowledgeBase, bool, error) {
	kb, err := s.upstream.GetKnowledgeBase(ctx, id, headers)
	if err == nil {
		return kb, true, nil
	}
	var upstreamError *weknora.Error
	if errors.As(err, &upstreamError) && upstreamError.Code == "upstream.not_found" {
		return weknora.KnowledgeBase{}, false, nil
	}
	return weknora.KnowledgeBase{}, false, translateUpstreamError(err)
}

func verifyUpstreamOwnership(kb weknora.KnowledgeBase, request CreationRequest, input CreateInput) error {
	if kb.ID != request.UpstreamKBID || kb.TenantID != request.TenantID || kb.CreatorID != request.OwnerUserID || kb.Name != input.Name || kb.Type != "document" {
		return &Error{Code: "space.reconciliation_conflict", Message: "The allocated upstream resource does not match this creation request", StatusCode: http.StatusConflict}
	}
	return nil
}

func (s *Service) ensureProfile(ctx context.Context, expected profile.Profile) error {
	_, err := s.profiles.Create(ctx, expected)
	if err == nil {
		return nil
	}
	if !errors.Is(err, profile.ErrConflict) {
		return &Error{Code: "space.profile_create_failed", Message: "Upstream resource exists but its product profile could not be created; retry with the same Idempotency-Key", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	existing, getErr := s.profiles.Get(ctx, expected.UpstreamKBID)
	if getErr != nil || existing.TenantID != expected.TenantID || existing.OwnerUserID != expected.OwnerUserID ||
		existing.ProductMode != expected.ProductMode || existing.AccessPolicy != expected.AccessPolicy ||
		existing.IndexProfile != expected.IndexProfile || existing.IndexProfileVersion != expected.IndexProfileVersion ||
		!jsonEqual(existing.EffectiveConfig, expected.EffectiveConfig) {
		return &Error{Code: "space.reconciliation_conflict", Message: "The upstream resource has a conflicting product profile", StatusCode: http.StatusConflict, Err: getErr}
	}
	return nil
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftJSON, _ := json.Marshal(leftValue)
	rightJSON, _ := json.Marshal(rightValue)
	return string(leftJSON) == string(rightJSON)
}

func (s *Service) recordFailure(ctx context.Context, request CreationRequest, err error) {
	message := "creation failed"
	var typed *Error
	if errors.As(err, &typed) {
		message = typed.Code
	}
	_ = s.requests.Fail(ctx, request, message)
}

func hashCreateInput(input CreateInput) (string, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func validateIdempotencyKey(value string) error {
	if len(value) < 8 || len(value) > 128 {
		return &Error{Code: "request.idempotency_key_required", Message: "Idempotency-Key must contain 8 to 128 characters", StatusCode: http.StatusBadRequest}
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return &Error{Code: "request.idempotency_key_invalid", Message: "Idempotency-Key contains unsupported characters", StatusCode: http.StatusBadRequest}
		}
	}
	return nil
}

func disabledModeError() error {
	return &Error{Code: "knowledge_mode.disabled", Message: "The requested knowledge mode or index profile is disabled", StatusCode: http.StatusBadRequest}
}

func translateUpstreamError(err error) error {
	var upstreamError *weknora.Error
	if errors.As(err, &upstreamError) {
		switch upstreamError.Code {
		case "upstream.not_found", "upstream.forbidden":
			return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound, Err: err}
		case "upstream.unauthorized":
			return &Error{Code: "auth.invalid", Message: "Authentication is invalid or expired", StatusCode: http.StatusUnauthorized, Err: err}
		}
	}
	return &Error{Code: "upstream.unavailable", Message: "Upstream service is unavailable", StatusCode: http.StatusBadGateway, Err: err}
}
