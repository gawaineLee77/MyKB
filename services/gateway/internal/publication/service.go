package publication

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
)

type OwnerResolver interface {
	Resolve(context.Context, string, http.Header) (ownership.Ownership, error)
}

type RevisionStore interface {
	Ensure(context.Context, string, string, string, string, string, time.Time) (int64, error)
	Current(context.Context, string) (int64, error)
}

type SubscriptionInvalidator interface {
	InactivatePublication(context.Context, string, string, time.Time) error
	InactivateOutsideAudience(context.Context, string, Audience, string, time.Time) error
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(service *Service) {
		if clock != nil {
			service.now = clock
		}
	}
}

func WithIDGenerator(generator func() (string, error)) Option {
	return func(service *Service) {
		if generator != nil {
			service.newID = generator
		}
	}
}

func WithAuditRecorder(recorder audit.Recorder) Option {
	return func(service *Service) { service.auditor = recorder }
}

func WithSubscriptionInvalidator(invalidator SubscriptionInvalidator) Option {
	return func(service *Service) { service.subscriptions = invalidator }
}

type Service struct {
	store         Store
	owners        OwnerResolver
	revisions     RevisionStore
	subscriptions SubscriptionInvalidator
	auditor       audit.Recorder
	now           func() time.Time
	newID         func() (string, error)
}

func NewService(store Store, owners OwnerResolver, revisions RevisionStore, options ...Option) (*Service, error) {
	if store == nil || owners == nil || revisions == nil {
		return nil, fmt.Errorf("publication store, owner resolver, and revision store are required")
	}
	service := &Service{store: store, owners: owners, revisions: revisions, now: time.Now, newID: newPublicationID}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) Publish(ctx context.Context, kbID string, actor Actor, request WriteRequest, headers http.Header) (Publication, error) {
	owner, err := s.requireOwner(ctx, kbID, actor, headers)
	if err != nil {
		return Publication{}, err
	}
	normalized, err := normalizeWrite(request)
	if err != nil {
		return Publication{}, err
	}
	now := s.now().UTC()
	revision, err := s.revisions.Ensure(ctx, kbID, actor.UserID, "publication.initialized", "", request.CorrelationID, now)
	if err != nil {
		return Publication{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	existing, lookupErr := s.store.GetByKB(ctx, kbID)
	if lookupErr == nil {
		if existing.Published() {
			return Publication{}, ErrConflict
		}
		if request.ExpectedRowVersion != existing.RowVersion {
			return Publication{}, ErrRevisionConflict
		}
		candidate := applyWrite(existing, normalized, revision, request.CorrelationID, now)
		candidate.Status = StatusPublished
		candidate.PublishedAt = now
		candidate.UnpublishedAt = nil
		updated, err := s.store.Update(ctx, candidate, existing.RowVersion)
		if err != nil {
			return Publication{}, err
		}
		if err := s.recordAudit(ctx, owner, actor, "publication.published", existing, updated, request.CorrelationID, now); err != nil {
			return Publication{}, err
		}
		return updated, nil
	}
	if !errors.Is(lookupErr, ErrNotFound) {
		return Publication{}, lookupErr
	}
	id, err := s.newID()
	if err != nil {
		return Publication{}, fmt.Errorf("generate publication ID: %w", err)
	}
	candidate := Publication{
		ID: id, KnowledgeBaseID: kbID, PublisherID: actor.UserID, PublisherTenantID: owner.TenantID,
		Title: normalized.Title, Description: normalized.Description, Tags: normalized.Tags,
		UsageGuidance: normalized.UsageGuidance, Audience: normalized.Audience, AccessMode: normalized.AccessMode,
		Status: StatusPublished, PublishedRevision: revision, CreatedAt: now, PublishedAt: now,
		UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: request.CorrelationID,
	}
	created, err := s.store.Create(ctx, candidate)
	if err != nil {
		return Publication{}, err
	}
	if err := s.recordAudit(ctx, owner, actor, "publication.published", Publication{}, created, request.CorrelationID, now); err != nil {
		return Publication{}, err
	}
	return created, nil
}

func (s *Service) Update(ctx context.Context, kbID string, actor Actor, request WriteRequest, headers http.Header) (Publication, error) {
	owner, err := s.requireOwner(ctx, kbID, actor, headers)
	if err != nil {
		return Publication{}, err
	}
	if request.ExpectedRowVersion < 1 {
		return Publication{}, ErrInvalid
	}
	normalized, err := normalizeWrite(request)
	if err != nil {
		return Publication{}, err
	}
	existing, err := s.store.GetByKB(ctx, kbID)
	if err != nil || !existing.Published() {
		if err == nil {
			err = ErrNotFound
		}
		return Publication{}, err
	}
	if existing.PublisherID != actor.UserID || existing.PublisherTenantID != actor.TenantID {
		return Publication{}, ErrNotOwner
	}
	if existing.RowVersion != request.ExpectedRowVersion {
		return Publication{}, ErrRevisionConflict
	}
	now := s.now().UTC()
	revision, err := s.revisions.Current(ctx, kbID)
	if err != nil {
		return Publication{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	candidate := applyWrite(existing, normalized, revision, request.CorrelationID, now)
	updated, err := s.store.Update(ctx, candidate, request.ExpectedRowVersion)
	if err != nil {
		return Publication{}, err
	}
	if s.subscriptions != nil {
		if err := s.subscriptions.InactivateOutsideAudience(ctx, updated.ID, updated.Audience, request.CorrelationID, now); err != nil {
			return Publication{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	if err := s.recordAudit(ctx, owner, actor, "publication.updated", existing, updated, request.CorrelationID, now); err != nil {
		return Publication{}, err
	}
	return updated, nil
}

func (s *Service) Unpublish(ctx context.Context, kbID string, actor Actor, expected int64, correlationID string, headers http.Header) (Publication, error) {
	owner, err := s.requireOwner(ctx, kbID, actor, headers)
	if err != nil {
		return Publication{}, err
	}
	if expected < 1 || !validCorrelation(correlationID) {
		return Publication{}, ErrInvalid
	}
	existing, err := s.store.GetByKB(ctx, kbID)
	if err != nil {
		return Publication{}, err
	}
	if existing.PublisherID != actor.UserID || existing.PublisherTenantID != actor.TenantID {
		return Publication{}, ErrNotOwner
	}
	if existing.RowVersion != expected {
		return Publication{}, ErrRevisionConflict
	}
	now := s.now().UTC()
	result := existing
	if existing.Published() {
		result, err = s.store.Unpublish(ctx, existing.ID, expected, correlationID, now)
		if err != nil {
			return Publication{}, err
		}
	}
	if s.subscriptions != nil {
		if err := s.subscriptions.InactivatePublication(ctx, result.ID, correlationID, now); err != nil {
			return Publication{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	if existing.Published() {
		if err := s.recordAudit(ctx, owner, actor, "publication.unpublished", existing, result, correlationID, now); err != nil {
			return Publication{}, err
		}
	}
	return result, nil
}

// UnpublishForDeletion removes publication-derived access before the upstream
// asynchronous KB deletion pipeline is allowed to start.
func (s *Service) UnpublishForDeletion(ctx context.Context, kbID string, actor Actor, correlationID string, headers http.Header) error {
	if _, err := s.requireOwner(ctx, kbID, actor, headers); err != nil {
		return err
	}
	existing, err := s.store.GetByKB(ctx, kbID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !existing.Published() {
		if s.subscriptions != nil {
			return s.subscriptions.InactivatePublication(ctx, existing.ID, correlationID, s.now().UTC())
		}
		return nil
	}
	_, err = s.Unpublish(ctx, kbID, actor, existing.RowVersion, correlationID, headers)
	return err
}

func (s *Service) GetForOwner(ctx context.Context, kbID string, actor Actor, headers http.Header) (Publication, error) {
	if _, err := s.requireOwner(ctx, kbID, actor, headers); err != nil {
		return Publication{}, err
	}
	return s.store.GetByKB(ctx, kbID)
}

func (s *Service) requireOwner(ctx context.Context, kbID string, actor Actor, headers http.Header) (ownership.Ownership, error) {
	if strings.TrimSpace(actor.UserID) == "" || actor.TenantID == 0 || strings.TrimSpace(kbID) == "" {
		return ownership.Ownership{}, ErrNotOwner
	}
	owner, err := s.owners.Resolve(ctx, kbID, headers)
	if err != nil {
		return ownership.Ownership{}, err
	}
	if actor.UserID != owner.OwnerUserID || actor.TenantID != owner.TenantID {
		s.recordDenied(ctx, owner, actor, headers)
		return ownership.Ownership{}, ErrNotOwner
	}
	if owner.IsPersonalNotes() {
		s.recordDenied(ctx, owner, actor, headers)
		return ownership.Ownership{}, ErrPersonalNotes
	}
	return owner, nil
}

func (s *Service) recordDenied(ctx context.Context, owner ownership.Ownership, actor Actor, headers http.Header) {
	if s.auditor == nil || actor.UserID == "" || actor.TenantID == 0 {
		return
	}
	correlationID := strings.TrimSpace(headers.Get("X-Request-ID"))
	if correlationID == "" {
		return
	}
	_ = s.auditor.Record(ctx, audit.Event{
		TenantID: owner.TenantID, KnowledgeBaseID: owner.KnowledgeBaseID, ActorUserID: actor.UserID,
		Action: "authorization.denied.publish", TargetType: "knowledge_base", TargetID: owner.KnowledgeBaseID,
		Outcome: audit.OutcomeDenied, ErrorCode: "resource.not_found", CorrelationID: correlationID,
		CreatedAt: s.now().UTC(),
	})
}

func normalizeWrite(request WriteRequest) (WriteRequest, error) {
	request.Title = strings.TrimSpace(request.Title)
	request.Description = strings.TrimSpace(request.Description)
	request.UsageGuidance = strings.TrimSpace(request.UsageGuidance)
	request.Audience = request.Audience.normalized()
	request.CorrelationID = strings.TrimSpace(request.CorrelationID)
	tags := make([]string, 0, len(request.Tags))
	seen := make(map[string]bool, len(request.Tags))
	for _, raw := range request.Tags {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	request.Tags = tags
	probe := Publication{
		ID: "probe", KnowledgeBaseID: "probe", PublisherID: "probe", PublisherTenantID: 1,
		Title: request.Title, Description: request.Description, Tags: request.Tags,
		UsageGuidance: request.UsageGuidance, Audience: request.Audience, AccessMode: request.AccessMode,
		Status: StatusPublished, PublishedRevision: 1, CreatedAt: time.Unix(1, 0), PublishedAt: time.Unix(1, 0),
		UpdatedAt: time.Unix(1, 0), RowVersion: 1, LastAuditCorrelationID: request.CorrelationID,
	}
	if err := probe.Validate(); err != nil || !validCorrelation(request.CorrelationID) {
		return WriteRequest{}, ErrInvalid
	}
	return request, nil
}

func applyWrite(existing Publication, request WriteRequest, revision int64, correlationID string, at time.Time) Publication {
	result := existing
	result.Title = request.Title
	result.Description = request.Description
	result.Tags = append([]string(nil), request.Tags...)
	result.UsageGuidance = request.UsageGuidance
	result.Audience = request.Audience
	result.AccessMode = request.AccessMode
	result.PublishedRevision = revision
	result.UpdatedAt = at
	result.LastAuditCorrelationID = correlationID
	return result
}

func validCorrelation(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 128
}

func (s *Service) recordAudit(ctx context.Context, owner ownership.Ownership, actor Actor, action string, oldValue, newValue Publication, correlationID string, at time.Time) error {
	if s.auditor == nil {
		return nil
	}
	var oldJSON, newJSON json.RawMessage
	if oldValue.ID != "" {
		oldJSON, _ = json.Marshal(publicationAuditValue(oldValue))
	}
	if newValue.ID != "" {
		newJSON, _ = json.Marshal(publicationAuditValue(newValue))
	}
	target := newValue.ID
	if target == "" {
		target = oldValue.ID
	}
	err := s.auditor.Record(ctx, audit.Event{
		TenantID: owner.TenantID, KnowledgeBaseID: owner.KnowledgeBaseID, ActorUserID: actor.UserID,
		Action: action, TargetType: "kb_publication", TargetID: target, Outcome: audit.OutcomeSuccess,
		CorrelationID: correlationID, OldValue: oldJSON, NewValue: newJSON, CreatedAt: at,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func publicationAuditValue(value Publication) any {
	return struct {
		Audience   Audience   `json:"audience"`
		AccessMode AccessMode `json:"access_mode"`
		Status     Status     `json:"status"`
		Revision   int64      `json:"published_revision"`
		RowVersion int64      `json:"row_version"`
	}{value.Audience, value.AccessMode, value.Status, value.PublishedRevision, value.RowVersion}
}

func newPublicationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
