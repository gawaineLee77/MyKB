// Package note implements owner-only Personal Notes operations over WeKnora manual knowledge.
package note

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/notespolicy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const (
	MaxPageSize    = 10
	MaxNoteBytes   = 64 << 10
	MaxNoteCount   = 500
	MaxCorpusBytes = 2 << 20
)

type Upstream interface {
	ListManualKnowledge(context.Context, string, int, int, http.Header) (weknora.ManualKnowledgePage, error)
	GetManualKnowledge(context.Context, string, string, http.Header) (weknora.ManualKnowledge, error)
	CreateManualKnowledge(context.Context, string, weknora.ManualKnowledgeInput, http.Header) (weknora.ManualKnowledge, error)
	UpdateManualKnowledge(context.Context, string, string, weknora.ManualKnowledgeInput, http.Header) (weknora.ManualKnowledge, error)
	DeleteManualKnowledge(context.Context, string, http.Header) error
}

type ProfileStore interface {
	Get(context.Context, string) (profile.Profile, error)
}

type Service struct {
	profiles  ProfileStore
	upstream  Upstream
	revisions RevisionStore
}

type WriteInput struct {
	Title           string `json:"title"`
	Content         string `json:"content"`
	Status          string `json:"status,omitempty"`
	ExpectedVersion *int   `json:"expected_version,omitempty"`
}

type RestoreInput struct {
	ExpectedVersion int `json:"expected_version"`
	TargetVersion   int `json:"target_version"`
}

type Note struct {
	ID              string    `json:"id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	Status          string    `json:"status"`
	Version         int       `json:"version"`
	ParseStatus     string    `json:"parse_status"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Summary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Version     int       `json:"version"`
	ContentSize int       `json:"content_size"`
	ParseStatus string    `json:"parse_status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Page struct {
	Items    []Summary `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
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

func NewService(profiles ProfileStore, upstream Upstream, revisions RevisionStore) (*Service, error) {
	if profiles == nil || upstream == nil || revisions == nil {
		return nil, fmt.Errorf("profile store, upstream adapter, and revision store are required")
	}
	return &Service{profiles: profiles, upstream: upstream, revisions: revisions}, nil
}

func (s *Service) List(ctx context.Context, kbID string, page, pageSize int, identity access.Identity, headers http.Header) (Page, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Read); err != nil {
		return Page{}, err
	}
	if page < 1 || pageSize < 1 || pageSize > MaxPageSize {
		return Page{}, invalid("Page and page_size are invalid")
	}
	result, err := s.upstream.ListManualKnowledge(ctx, kbID, page, pageSize, headers)
	if err != nil {
		return Page{}, translateUpstream(err)
	}
	items := make([]Summary, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, Summary{
			ID: item.ID, Title: item.Title, Status: item.Metadata.Status, Version: item.Metadata.Version,
			ContentSize: len([]byte(item.Metadata.Content)), ParseStatus: item.ParseStatus, UpdatedAt: item.UpdatedAt,
		})
	}
	return Page{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, kbID, noteID string, identity access.Identity, headers http.Header) (Note, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Read); err != nil {
		return Note{}, err
	}
	item, err := s.upstream.GetManualKnowledge(ctx, kbID, noteID, headers)
	if err != nil {
		return Note{}, translateUpstream(err)
	}
	if _, err := s.recordRevision(ctx, item, "snapshot", identity.UserID, nil); err != nil {
		return Note{}, err
	}
	return noteFromUpstream(item), nil
}

func (s *Service) Create(ctx context.Context, kbID string, input WriteInput, identity access.Identity, headers http.Header) (Note, error) {
	return s.create(ctx, kbID, input, "create", identity, headers)
}

func (s *Service) create(ctx context.Context, kbID string, input WriteInput, operation string, identity access.Identity, headers http.Header) (Note, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Write); err != nil {
		return Note{}, err
	}
	normalized, err := normalizeWrite(input)
	if err != nil {
		return Note{}, err
	}
	if err := s.enforceQuota(ctx, kbID, len([]byte(normalized.Content)), 0, false, headers); err != nil {
		return Note{}, err
	}
	item, err := s.upstream.CreateManualKnowledge(ctx, kbID, manualInput(normalized), headers)
	if err != nil {
		return Note{}, translateUpstream(err)
	}
	if _, err := s.recordRevision(ctx, item, operation, identity.UserID, nil); err != nil {
		_ = s.upstream.DeleteManualKnowledge(ctx, item.ID, headers)
		return Note{}, err
	}
	return noteFromUpstream(item), nil
}

func (s *Service) Update(ctx context.Context, kbID, noteID string, input WriteInput, identity access.Identity, headers http.Header) (Note, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Write); err != nil {
		return Note{}, err
	}
	existing, err := s.upstream.GetManualKnowledge(ctx, kbID, noteID, headers)
	if err != nil {
		return Note{}, translateUpstream(err)
	}
	if _, err := s.recordRevision(ctx, existing, "snapshot", identity.UserID, nil); err != nil {
		return Note{}, err
	}
	if input.ExpectedVersion == nil || *input.ExpectedVersion != existing.Metadata.Version {
		return Note{}, &Error{Code: "note.version_conflict", Message: "The note changed after it was opened; reload before saving", StatusCode: http.StatusConflict}
	}
	normalized, err := normalizeWrite(input)
	if err != nil {
		return Note{}, err
	}
	if err := s.enforceQuota(ctx, kbID, len([]byte(normalized.Content)), len([]byte(existing.Metadata.Content)), true, headers); err != nil {
		return Note{}, err
	}
	item, err := s.upstream.UpdateManualKnowledge(ctx, kbID, noteID, manualInput(normalized), headers)
	if err != nil {
		return Note{}, translateUpstream(err)
	}
	if _, err := s.recordRevision(ctx, item, "edit", identity.UserID, nil); err != nil {
		return Note{}, err
	}
	return noteFromUpstream(item), nil
}

func (s *Service) Import(ctx context.Context, kbID, filename string, content []byte, identity access.Identity, headers http.Header) (Note, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Write); err != nil {
		return Note{}, err
	}
	base := filepath.Base(strings.TrimSpace(filename))
	extension := strings.ToLower(filepath.Ext(base))
	if base == "." || base == "" || (extension != ".md" && extension != ".txt") {
		return Note{}, &Error{Code: "note.file_type_unsupported", Message: "Personal Notes imports accept only .md and .txt files", StatusCode: http.StatusUnsupportedMediaType}
	}
	if !utf8.Valid(content) {
		return Note{}, &Error{Code: "note.invalid_utf8", Message: "Note files must use valid UTF-8 text", StatusCode: http.StatusBadRequest}
	}
	title := strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	return s.create(ctx, kbID, WriteInput{Title: title, Content: string(content), Status: "publish"}, "import", identity, headers)
}

func (s *Service) ListRevisions(ctx context.Context, kbID, noteID string, identity access.Identity) ([]Revision, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Read); err != nil {
		return nil, err
	}
	result, err := s.revisions.List(ctx, kbID, noteID)
	if err != nil {
		return nil, revisionStateError(err)
	}
	return result, nil
}

func (s *Service) GetRevision(ctx context.Context, kbID, noteID string, version int, identity access.Identity) (Revision, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Read); err != nil {
		return Revision{}, err
	}
	result, err := s.revisions.Get(ctx, kbID, noteID, version)
	if errors.Is(err, ErrRevisionNotFound) {
		return Revision{}, &Error{Code: "note.revision_not_found", Message: "Note revision not found", StatusCode: http.StatusNotFound}
	}
	if err != nil {
		return Revision{}, revisionStateError(err)
	}
	return result, nil
}

func (s *Service) Restore(ctx context.Context, kbID, noteID string, input RestoreInput, identity access.Identity, headers http.Header) (Note, error) {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Write); err != nil {
		return Note{}, err
	}
	current, err := s.upstream.GetManualKnowledge(ctx, kbID, noteID, headers)
	if err != nil {
		return Note{}, translateUpstream(err)
	}
	if _, err := s.recordRevision(ctx, current, "snapshot", identity.UserID, nil); err != nil {
		return Note{}, err
	}
	if input.ExpectedVersion < 1 || input.ExpectedVersion != current.Metadata.Version {
		return Note{}, &Error{Code: "note.version_conflict", Message: "The note changed after it was opened; reload before restoring", StatusCode: http.StatusConflict}
	}
	if input.TargetVersion < 1 || input.TargetVersion >= current.Metadata.Version {
		return Note{}, invalid("Target revision must be older than the current note")
	}
	target, err := s.revisions.Get(ctx, kbID, noteID, input.TargetVersion)
	if errors.Is(err, ErrRevisionNotFound) {
		return Note{}, &Error{Code: "note.revision_not_found", Message: "Note revision not found", StatusCode: http.StatusNotFound}
	}
	if err != nil {
		return Note{}, revisionStateError(err)
	}
	if err := s.enforceQuota(ctx, kbID, len([]byte(target.Content)), len([]byte(current.Metadata.Content)), true, headers); err != nil {
		return Note{}, err
	}
	item, err := s.upstream.UpdateManualKnowledge(ctx, kbID, noteID, weknora.ManualKnowledgeInput{
		Title: target.Title, Content: target.Content, Status: target.Status, Channel: "web",
	}, headers)
	if err != nil {
		return Note{}, translateUpstream(err)
	}
	if _, err := s.recordRevision(ctx, item, "restore", identity.UserID, &target.Version); err != nil {
		return Note{}, err
	}
	return noteFromUpstream(item), nil
}

func (s *Service) Delete(ctx context.Context, kbID, noteID string, identity access.Identity, headers http.Header) error {
	if err := s.authorize(ctx, kbID, identity, notespolicy.Write); err != nil {
		return err
	}
	if _, err := s.upstream.GetManualKnowledge(ctx, kbID, noteID, headers); err != nil {
		return translateUpstream(err)
	}
	if err := s.upstream.DeleteManualKnowledge(ctx, noteID, headers); err != nil {
		return translateUpstream(err)
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, kbID string, identity access.Identity, action notespolicy.Operation) error {
	if strings.TrimSpace(kbID) == "" {
		return invalid("Knowledge base ID is required")
	}
	productProfile, err := s.profiles.Get(ctx, kbID)
	if errors.Is(err, profile.ErrNotFound) {
		return &Error{Code: "note_space.not_found", Message: "Personal Notes space not found", StatusCode: http.StatusNotFound}
	}
	if err != nil {
		return &Error{Code: "note.state_unavailable", Message: "Notes state is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	if productProfile.ProductMode != profile.ModePersonalNotes {
		return &Error{Code: "note_space.mode_required", Message: "This operation requires a Personal Notes space", StatusCode: http.StatusConflict}
	}
	if err := notespolicy.Authorize(productProfile, notespolicy.Principal{UserID: identity.UserID, TenantID: identity.TenantID}, action); err != nil {
		var policyError *notespolicy.Error
		if errors.As(err, &policyError) {
			return &Error{Code: policyError.Code, Message: policyError.Message, StatusCode: policyError.StatusCode, Err: err}
		}
		return err
	}
	return nil
}

func normalizeWrite(input WriteInput) (WriteInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.ReplaceAll(input.Content, "\r\n", "\n")
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "publish"
	}
	if input.Title == "" || strings.TrimSpace(input.Content) == "" || (input.Status != "draft" && input.Status != "publish") {
		return input, invalid("Title, content, or status is invalid")
	}
	if !utf8.ValidString(input.Title) || !utf8.ValidString(input.Content) {
		return input, &Error{Code: "note.invalid_utf8", Message: "Notes must use valid UTF-8 text", StatusCode: http.StatusBadRequest}
	}
	if len([]byte(input.Content)) > MaxNoteBytes {
		return input, &Error{Code: "note.size_quota_exceeded", Message: "A note cannot exceed 64 KiB", StatusCode: http.StatusRequestEntityTooLarge}
	}
	return input, nil
}

func (s *Service) enforceQuota(ctx context.Context, kbID string, newBytes, replacedBytes int, replacing bool, headers http.Header) error {
	first, err := s.upstream.ListManualKnowledge(ctx, kbID, 1, MaxPageSize, headers)
	if err != nil {
		return translateUpstream(err)
	}
	if !replacing && first.Total >= MaxNoteCount {
		return &Error{Code: "note.count_quota_exceeded", Message: "A Personal Notes space can contain at most 500 notes", StatusCode: http.StatusConflict}
	}
	totalBytes := 0
	page := first
	seen := 0
	for {
		for _, item := range page.Items {
			totalBytes += len([]byte(item.Metadata.Content))
			seen++
		}
		if seen >= page.Total {
			break
		}
		if len(page.Items) == 0 {
			return &Error{Code: "note.upstream_failed", Message: "Unable to calculate Note Space quota", StatusCode: http.StatusBadGateway}
		}
		page, err = s.upstream.ListManualKnowledge(ctx, kbID, page.Page+1, MaxPageSize, headers)
		if err != nil {
			return translateUpstream(err)
		}
	}
	projected := totalBytes + newBytes
	if replacing {
		projected -= replacedBytes
	}
	if projected > MaxCorpusBytes {
		return &Error{Code: "note.corpus_quota_exceeded", Message: "A Personal Notes space cannot exceed 2 MiB", StatusCode: http.StatusConflict}
	}
	return nil
}

func manualInput(input WriteInput) weknora.ManualKnowledgeInput {
	return weknora.ManualKnowledgeInput{Title: input.Title, Content: input.Content, Status: input.Status, Channel: "web"}
}

func (s *Service) recordRevision(ctx context.Context, item weknora.ManualKnowledge, operation, actor string, restoredFrom *int) (Revision, error) {
	revision, err := s.revisions.Record(ctx, Revision{
		KnowledgeBaseID: item.KnowledgeBaseID, NoteID: item.ID, Version: item.Metadata.Version,
		Title: item.Title, Content: item.Metadata.Content, Status: item.Metadata.Status,
		Operation: operation, RestoredFromVersion: restoredFrom, ActorUserID: actor,
	})
	if err != nil {
		return Revision{}, revisionStateError(err)
	}
	return revision, nil
}

func revisionStateError(err error) error {
	return &Error{Code: "note.revision_state_unavailable", Message: "Note revision history is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
}

func noteFromUpstream(item weknora.ManualKnowledge) Note {
	return Note{
		ID: item.ID, KnowledgeBaseID: item.KnowledgeBaseID, Title: item.Title, Content: item.Metadata.Content,
		Status: item.Metadata.Status, Version: item.Metadata.Version, ParseStatus: item.ParseStatus,
		ErrorMessage: item.ErrorMessage, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func invalid(message string) error {
	return &Error{Code: "note.invalid_request", Message: message, StatusCode: http.StatusBadRequest}
}

func translateUpstream(err error) error {
	var upstreamError *weknora.Error
	if !errors.As(err, &upstreamError) {
		return &Error{Code: "note.upstream_failed", Message: "Notes operation failed", StatusCode: http.StatusBadGateway, Err: err}
	}
	code, message := "note.upstream_failed", "Notes operation failed"
	if upstreamError.Code == "upstream.not_found" {
		code, message = "note.not_found", "Note not found"
	}
	return &Error{Code: code, Message: message, StatusCode: upstreamError.StatusCode, Err: err}
}
