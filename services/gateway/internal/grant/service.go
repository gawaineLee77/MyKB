package grant

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
)

type Actor struct {
	UserID   string
	TenantID uint64
}

type CreateRequest struct {
	SubjectType   SubjectType
	SubjectID     string
	Permission    Permission
	ExpiresAt     *time.Time
	CorrelationID string
}

type UpdateRequest struct {
	ExpectedRevision int64
	Permission       Permission
	ExpiresAt        *time.Time
	CorrelationID    string
}

type RevokeRequest struct {
	ExpectedRevision int64
	CorrelationID    string
}

type OwnerResolver interface {
	Resolve(context.Context, string, http.Header) (ownership.Ownership, error)
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
	return func(service *Service) {
		service.auditor = recorder
	}
}

type Service struct {
	store   Store
	owners  OwnerResolver
	now     func() time.Time
	newID   func() (string, error)
	auditor audit.Recorder
}

func NewService(store Store, owners OwnerResolver, options ...Option) (*Service, error) {
	if store == nil || owners == nil {
		return nil, fmt.Errorf("grant store and owner resolver are required")
	}
	service := &Service{store: store, owners: owners, now: time.Now, newID: newGrantID}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) Create(ctx context.Context, kbID string, actor Actor, request CreateRequest, inbound http.Header) (Grant, error) {
	owner, err := s.requireOwner(ctx, kbID, actor, inbound)
	if err != nil {
		return Grant{}, err
	}
	now := s.now().UTC()
	if err := validateCreateRequest(request, owner, now); err != nil {
		return Grant{}, err
	}
	existing, err := s.store.FindCurrentBySubject(ctx, kbID, request.SubjectType, request.SubjectID)
	if err == nil {
		if sameGrantRequest(existing, request) {
			return existing, nil
		}
		return Grant{}, ErrConflict
	}
	if !errors.Is(err, ErrNotFound) {
		return Grant{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Grant{}, fmt.Errorf("generate grant ID: %w", err)
	}
	candidate := Grant{
		ID: id, KnowledgeBaseID: kbID, SubjectType: request.SubjectType, SubjectID: request.SubjectID,
		Permission: request.Permission, GrantedBy: actor.UserID, CreatedAt: now, UpdatedAt: now,
		ExpiresAt: request.ExpiresAt, Revision: 1, LastAuditCorrelationID: request.CorrelationID,
	}
	created, err := s.store.Create(ctx, candidate)
	if err == nil {
		if err := s.recordAudit(ctx, owner, actor, "grant.create", Grant{}, created, request.CorrelationID, now); err != nil {
			return Grant{}, err
		}
		return created, nil
	}
	if !errors.Is(err, ErrConflict) {
		return created, err
	}
	// A concurrent retry may have won the active-subject uniqueness race.
	existing, lookupErr := s.store.FindCurrentBySubject(ctx, kbID, request.SubjectType, request.SubjectID)
	if lookupErr == nil && sameGrantRequest(existing, request) {
		return existing, nil
	}
	return Grant{}, ErrConflict
}

func (s *Service) List(ctx context.Context, kbID string, actor Actor, inbound http.Header) ([]Grant, error) {
	if _, err := s.requireOwner(ctx, kbID, actor, inbound); err != nil {
		return nil, err
	}
	items, err := s.store.ListCurrentByKB(ctx, kbID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	active := make([]Grant, 0, len(items))
	for _, item := range items {
		if item.Active(now) {
			active = append(active, item)
		}
	}
	return active, nil
}

func (s *Service) Update(ctx context.Context, kbID, grantID string, actor Actor, request UpdateRequest, inbound http.Header) (Grant, error) {
	owner, err := s.requireOwner(ctx, kbID, actor, inbound)
	if err != nil {
		return Grant{}, err
	}
	now := s.now().UTC()
	if grantID == "" || request.ExpectedRevision < 1 || !request.Permission.Valid() ||
		!validCorrelationID(request.CorrelationID) || (request.ExpiresAt != nil && !request.ExpiresAt.After(now)) {
		return Grant{}, ErrInvalid
	}
	existing, err := s.store.Get(ctx, grantID)
	if err != nil {
		return Grant{}, err
	}
	if existing.KnowledgeBaseID != kbID || existing.RevokedAt != nil {
		return Grant{}, ErrNotFound
	}
	if existing.Revision != request.ExpectedRevision {
		return Grant{}, ErrRevisionConflict
	}
	updated, err := s.store.Update(ctx, grantID, request.ExpectedRevision, request.Permission, request.ExpiresAt, request.CorrelationID, now)
	if err != nil {
		return Grant{}, err
	}
	if err := s.recordAudit(ctx, owner, actor, "grant.update", existing, updated, request.CorrelationID, now); err != nil {
		return Grant{}, err
	}
	return updated, nil
}

func (s *Service) Revoke(ctx context.Context, kbID, grantID string, actor Actor, request RevokeRequest, inbound http.Header) (Grant, error) {
	owner, err := s.requireOwner(ctx, kbID, actor, inbound)
	if err != nil {
		return Grant{}, err
	}
	if grantID == "" || request.ExpectedRevision < 1 || !validCorrelationID(request.CorrelationID) {
		return Grant{}, ErrInvalid
	}
	existing, err := s.store.Get(ctx, grantID)
	if err != nil {
		return Grant{}, err
	}
	if existing.KnowledgeBaseID != kbID || existing.RevokedAt != nil {
		return Grant{}, ErrNotFound
	}
	if existing.Revision != request.ExpectedRevision {
		return Grant{}, ErrRevisionConflict
	}
	now := s.now().UTC()
	revoked, err := s.store.Revoke(ctx, grantID, request.ExpectedRevision, request.CorrelationID, now)
	if err != nil {
		return Grant{}, err
	}
	if err := s.recordAudit(ctx, owner, actor, "grant.revoke", existing, revoked, request.CorrelationID, now); err != nil {
		return Grant{}, err
	}
	return revoked, nil
}

func (s *Service) recordAudit(ctx context.Context, owner ownership.Ownership, actor Actor, action string, oldGrant, newGrant Grant, correlationID string, at time.Time) error {
	if s.auditor == nil {
		return nil
	}
	var oldValue, newValue json.RawMessage
	if oldGrant.ID != "" {
		oldValue, _ = json.Marshal(auditGrantValue(oldGrant))
	}
	if newGrant.ID != "" {
		newValue, _ = json.Marshal(auditGrantValue(newGrant))
	}
	targetID := newGrant.ID
	if targetID == "" {
		targetID = oldGrant.ID
	}
	err := s.auditor.Record(ctx, audit.Event{
		TenantID: owner.TenantID, KnowledgeBaseID: owner.KnowledgeBaseID, ActorUserID: actor.UserID,
		Action: action, TargetType: "kb_grant", TargetID: targetID, Outcome: audit.OutcomeSuccess,
		CorrelationID: correlationID, OldValue: oldValue, NewValue: newValue, CreatedAt: at,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	return nil
}

type grantAuditValue struct {
	SubjectType SubjectType `json:"subject_type"`
	SubjectID   string      `json:"subject_id"`
	Permission  Permission  `json:"permission"`
	ExpiresAt   *time.Time  `json:"expires_at,omitempty"`
	RevokedAt   *time.Time  `json:"revoked_at,omitempty"`
	Revision    int64       `json:"revision"`
}

func auditGrantValue(value Grant) grantAuditValue {
	return grantAuditValue{
		SubjectType: value.SubjectType, SubjectID: value.SubjectID, Permission: value.Permission,
		ExpiresAt: value.ExpiresAt, RevokedAt: value.RevokedAt, Revision: value.Revision,
	}
}

func (s *Service) requireOwner(ctx context.Context, kbID string, actor Actor, inbound http.Header) (ownership.Ownership, error) {
	if strings.TrimSpace(actor.UserID) == "" || actor.TenantID == 0 {
		return ownership.Ownership{}, ErrNotOwner
	}
	owner, err := s.owners.Resolve(ctx, kbID, inbound)
	if err != nil {
		return ownership.Ownership{}, err
	}
	if actor.UserID != owner.OwnerUserID || actor.TenantID != owner.TenantID {
		s.recordDeniedGrant(ctx, owner, actor, inbound)
		return ownership.Ownership{}, ErrNotOwner
	}
	if owner.IsPersonalNotes() {
		s.recordDeniedGrant(ctx, owner, actor, inbound)
		return ownership.Ownership{}, ErrPersonalNotes
	}
	return owner, nil
}

func (s *Service) recordDeniedGrant(ctx context.Context, owner ownership.Ownership, actor Actor, headers http.Header) {
	if s.auditor == nil || strings.TrimSpace(actor.UserID) == "" || actor.TenantID == 0 {
		return
	}
	correlationID := strings.TrimSpace(headers.Get("X-Request-ID"))
	if correlationID == "" {
		return
	}
	_ = s.auditor.Record(ctx, audit.Event{
		TenantID: owner.TenantID, KnowledgeBaseID: owner.KnowledgeBaseID, ActorUserID: actor.UserID,
		Action: "authorization.denied.manage_grants", TargetType: "knowledge_base", TargetID: owner.KnowledgeBaseID,
		Outcome: audit.OutcomeDenied, ErrorCode: "resource.not_found", CorrelationID: correlationID,
		CreatedAt: s.now().UTC(),
	})
}

func validateCreateRequest(request CreateRequest, owner ownership.Ownership, now time.Time) error {
	if request.SubjectType != SubjectUser {
		if request.SubjectType.Valid() {
			return ErrSubjectUnsupported
		}
		return ErrInvalid
	}
	if strings.TrimSpace(request.SubjectID) == "" || len(request.SubjectID) > 36 || request.SubjectID == owner.OwnerUserID ||
		!request.Permission.Valid() || !validCorrelationID(request.CorrelationID) ||
		(request.ExpiresAt != nil && !request.ExpiresAt.After(now)) {
		return ErrInvalid
	}
	return nil
}

func validCorrelationID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 128
}

func sameGrantRequest(existing Grant, request CreateRequest) bool {
	return existing.SubjectType == request.SubjectType && existing.SubjectID == request.SubjectID &&
		existing.Permission == request.Permission && sameTime(existing.ExpiresAt, request.ExpiresAt)
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func newGrantID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
