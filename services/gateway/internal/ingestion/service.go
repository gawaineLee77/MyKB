// Package ingestion exposes the approved Plain RAG document lifecycle.
package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/preset"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const MaxPageSize = 100

var allowedExtensions = map[string]struct{}{
	".md": {}, ".txt": {}, ".pdf": {}, ".doc": {}, ".docx": {}, ".xls": {}, ".xlsx": {},
	".ppt": {}, ".pptx": {}, ".csv": {}, ".html": {}, ".htm": {}, ".json": {}, ".xml": {},
}

type Upstream interface {
	GetKnowledgeBase(context.Context, string, http.Header) (weknora.KnowledgeBase, error)
	UploadKnowledge(context.Context, string, string, io.Reader, http.Header) (weknora.Knowledge, error)
	ListKnowledge(context.Context, string, int, int, http.Header) (weknora.KnowledgePage, error)
	GetKnowledge(context.Context, string, string, http.Header) (weknora.Knowledge, error)
	ReparseKnowledge(context.Context, string, string, http.Header) (weknora.Knowledge, error)
	CancelKnowledge(context.Context, string, string, http.Header) (weknora.Knowledge, error)
}

type ProfileStore interface {
	Get(context.Context, string) (profile.Profile, error)
}

type Service struct {
	profiles ProfileStore
	upstream Upstream
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

func NewService(profiles ProfileStore, upstream Upstream) (*Service, error) {
	if profiles == nil || upstream == nil {
		return nil, fmt.Errorf("profile store and upstream adapter are required")
	}
	return &Service{profiles: profiles, upstream: upstream}, nil
}

func (s *Service) Upload(ctx context.Context, kbID, filename string, size int64, source io.Reader, identity access.Identity, headers http.Header) (weknora.Knowledge, error) {
	config, err := s.authorize(ctx, kbID, identity, headers)
	if err != nil {
		return weknora.Knowledge{}, err
	}
	base := filepath.Base(strings.TrimSpace(filename))
	if _, ok := allowedExtensions[strings.ToLower(filepath.Ext(base))]; !ok || base == "." {
		return weknora.Knowledge{}, &Error{Code: "ingestion.file_type_unsupported", Message: "File type is not supported by the Plain RAG profile", StatusCode: http.StatusUnsupportedMediaType}
	}
	if size < 1 || size > config.Limits.MaxFileBytes {
		return weknora.Knowledge{}, &Error{Code: "ingestion.file_size_exceeded", Message: "File exceeds the Plain RAG profile limit", StatusCode: http.StatusRequestEntityTooLarge}
	}
	result, err := s.upstream.UploadKnowledge(ctx, kbID, base, source, headers)
	if err != nil {
		return weknora.Knowledge{}, translateUpstream(err)
	}
	return result, nil
}

func (s *Service) List(ctx context.Context, kbID string, page, pageSize int, identity access.Identity, headers http.Header) (weknora.KnowledgePage, error) {
	if _, err := s.authorize(ctx, kbID, identity, headers); err != nil {
		return weknora.KnowledgePage{}, err
	}
	if page < 1 || pageSize < 1 || pageSize > MaxPageSize {
		return weknora.KnowledgePage{}, invalid("Page or page_size is invalid")
	}
	result, err := s.upstream.ListKnowledge(ctx, kbID, page, pageSize, headers)
	if err != nil {
		return weknora.KnowledgePage{}, translateUpstream(err)
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, kbID, knowledgeID string, identity access.Identity, headers http.Header) (weknora.Knowledge, error) {
	if _, err := s.authorize(ctx, kbID, identity, headers); err != nil {
		return weknora.Knowledge{}, err
	}
	result, err := s.upstream.GetKnowledge(ctx, kbID, knowledgeID, headers)
	if err != nil {
		return weknora.Knowledge{}, translateUpstream(err)
	}
	return result, nil
}

func (s *Service) Retry(ctx context.Context, kbID, knowledgeID string, identity access.Identity, headers http.Header) (weknora.Knowledge, error) {
	if _, err := s.Get(ctx, kbID, knowledgeID, identity, headers); err != nil {
		return weknora.Knowledge{}, err
	}
	result, err := s.upstream.ReparseKnowledge(ctx, kbID, knowledgeID, headers)
	if err != nil {
		return weknora.Knowledge{}, translateUpstream(err)
	}
	return result, nil
}

func (s *Service) Cancel(ctx context.Context, kbID, knowledgeID string, identity access.Identity, headers http.Header) (weknora.Knowledge, error) {
	if _, err := s.Get(ctx, kbID, knowledgeID, identity, headers); err != nil {
		return weknora.Knowledge{}, err
	}
	result, err := s.upstream.CancelKnowledge(ctx, kbID, knowledgeID, headers)
	if err != nil {
		return weknora.Knowledge{}, translateUpstream(err)
	}
	return result, nil
}

func (s *Service) authorize(ctx context.Context, kbID string, identity access.Identity, headers http.Header) (preset.EffectiveConfig, error) {
	if strings.TrimSpace(kbID) == "" || identity.UserID == "" || identity.TenantID == 0 {
		return preset.EffectiveConfig{}, invalid("Knowledge base and authenticated principal are required")
	}
	if _, err := s.upstream.GetKnowledgeBase(ctx, kbID, headers); err != nil {
		return preset.EffectiveConfig{}, translateUpstream(err)
	}
	productProfile, err := s.profiles.Get(ctx, kbID)
	if errors.Is(err, profile.ErrNotFound) {
		return preset.EffectiveConfig{}, &Error{Code: "rag.profile_not_found", Message: "Plain RAG profile not found", StatusCode: http.StatusNotFound}
	}
	if err != nil {
		return preset.EffectiveConfig{}, &Error{Code: "ingestion.state_unavailable", Message: "Plain RAG state is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	if productProfile.ProductMode != profile.ModeRAG || productProfile.IndexProfile != "plain" || productProfile.IndexProfileVersion != preset.Version {
		return preset.EffectiveConfig{}, &Error{Code: "rag.plain_profile_required", Message: "This operation requires the approved Plain RAG profile", StatusCode: http.StatusConflict}
	}
	var config preset.EffectiveConfig
	if err := json.Unmarshal(productProfile.EffectiveConfig, &config); err != nil || config.ProfileID != "plain" || config.ProfileVersion != preset.Version ||
		!config.Indexing.VectorEnabled || !config.Indexing.KeywordEnabled || config.Indexing.GraphEnabled || config.Indexing.WikiEnabled ||
		config.Storage.Provider != "local" || config.Limits.MaxFileBytes < 1 {
		return preset.EffectiveConfig{}, &Error{Code: "rag.profile_invalid", Message: "Stored Plain RAG profile is invalid", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	return config, nil
}

func invalid(message string) error {
	return &Error{Code: "ingestion.invalid_request", Message: message, StatusCode: http.StatusBadRequest}
}

func translateUpstream(err error) error {
	var upstreamError *weknora.Error
	if errors.As(err, &upstreamError) {
		if upstreamError.Code == "upstream.not_found" || upstreamError.Code == "upstream.forbidden" {
			return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound, Err: err}
		}
		if upstreamError.Code == "upstream.unauthorized" {
			return &Error{Code: "auth.invalid", Message: "Authentication is invalid or expired", StatusCode: http.StatusUnauthorized, Err: err}
		}
		return &Error{Code: "ingestion.upstream_rejected", Message: "Document ingestion was rejected", StatusCode: upstreamError.StatusCode, Err: err}
	}
	return &Error{Code: "ingestion.upstream_failed", Message: "Document ingestion failed", StatusCode: http.StatusBadGateway, Err: err}
}
