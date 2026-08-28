package subscription

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
)

type PublicationReader interface {
	Get(context.Context, string) (publication.Publication, error)
}

type RevisionReader interface {
	Current(context.Context, string) (int64, error)
}

type Item struct {
	Subscription    Subscription            `json:"subscription"`
	Publication     publication.Publication `json:"publication"`
	CurrentRevision int64                   `json:"current_revision"`
	Updated         bool                    `json:"updated"`
}

type Result struct {
	Item    Item `json:"item"`
	Changed bool `json:"changed"`
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

type Service struct {
	store     Store
	pubs      PublicationReader
	revisions RevisionReader
	auditor   audit.Recorder
	now       func() time.Time
	newID     func() (string, error)
}

func NewService(store Store, pubs PublicationReader, revisions RevisionReader, options ...Option) (*Service, error) {
	if store == nil || pubs == nil || revisions == nil {
		return nil, fmt.Errorf("subscription store, publication reader, and revision reader are required")
	}
	service := &Service{store: store, pubs: pubs, revisions: revisions, now: time.Now, newID: newSubscriptionID}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) Subscribe(ctx context.Context, publicationID string, actor Actor, correlationID string) (Result, error) {
	if !validActor(actor) || !validCorrelation(correlationID) || strings.TrimSpace(publicationID) == "" {
		return Result{}, ErrInvalid
	}
	pub, err := s.pubs.Get(ctx, publicationID)
	if err != nil || !pub.Published() {
		return Result{}, ErrPublication
	}
	if pub.PublisherID == actor.UserID {
		return Result{}, ErrOwner
	}
	if !pub.VisibleTo(actor.TenantID) {
		return Result{}, ErrOutsideAudience
	}
	current, err := s.revisions.Current(ctx, pub.KnowledgeBaseID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	existing, err := s.store.GetByPublicationUser(ctx, publicationID, actor.UserID)
	if err == nil && existing.Active() && existing.SubscriberTenantID == actor.TenantID {
		return Result{Item: item(existing, pub, current), Changed: false}, nil
	}
	now := s.now().UTC()
	var active Subscription
	if err == nil {
		active, err = s.store.Activate(ctx, existing.ID, actor.TenantID, current, correlationID, now)
	} else if errors.Is(err, ErrNotFound) {
		id, idErr := s.newID()
		if idErr != nil {
			return Result{}, idErr
		}
		active, err = s.store.Create(ctx, Subscription{
			ID: id, PublicationID: publicationID, SubscriberID: actor.UserID, SubscriberTenantID: actor.TenantID,
			Status: StatusActive, NotificationEnabled: true, LastSeenRevision: current,
			CreatedAt: now, UpdatedAt: now, LastAuditCorrelationID: correlationID,
		})
	}
	if errors.Is(err, ErrInvalid) {
		winner, readErr := s.store.GetByPublicationUser(ctx, publicationID, actor.UserID)
		if readErr == nil && winner.Active() && winner.SubscriberTenantID == actor.TenantID {
			return Result{Item: item(winner, pub, current), Changed: false}, nil
		}
	}
	if err != nil {
		return Result{}, err
	}
	if err := s.recordAudit(ctx, pub, actor, "subscription.created", existing, active, correlationID, now); err != nil {
		return Result{}, err
	}
	return Result{Item: item(active, pub, current), Changed: true}, nil
}

func (s *Service) Unsubscribe(ctx context.Context, publicationID string, actor Actor, correlationID string) (Result, error) {
	if !validActor(actor) || !validCorrelation(correlationID) || publicationID == "" {
		return Result{}, ErrInvalid
	}
	pub, err := s.pubs.Get(ctx, publicationID)
	if err != nil {
		return Result{}, ErrPublication
	}
	existing, err := s.store.GetByPublicationUser(ctx, publicationID, actor.UserID)
	if errors.Is(err, ErrNotFound) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if existing.SubscriberTenantID != actor.TenantID {
		return Result{}, ErrNotFound
	}
	current, err := s.currentOrPublished(ctx, pub)
	if err != nil {
		return Result{}, err
	}
	if existing.Status == StatusUnsubscribed {
		return Result{Item: item(existing, pub, current), Changed: false}, nil
	}
	now := s.now().UTC()
	ended, err := s.store.Unsubscribe(ctx, existing.ID, correlationID, now)
	if err != nil {
		return Result{}, err
	}
	if err := s.recordAudit(ctx, pub, actor, "subscription.ended", existing, ended, correlationID, now); err != nil {
		return Result{}, err
	}
	return Result{Item: item(ended, pub, current), Changed: true}, nil
}

func (s *Service) MarkSeen(ctx context.Context, publicationID string, actor Actor, correlationID string) (Result, error) {
	if !validActor(actor) || !validCorrelation(correlationID) || publicationID == "" {
		return Result{}, ErrInvalid
	}
	pub, err := s.pubs.Get(ctx, publicationID)
	if err != nil || !pub.VisibleTo(actor.TenantID) {
		return Result{}, ErrPublication
	}
	existing, err := s.store.GetByPublicationUser(ctx, publicationID, actor.UserID)
	if err != nil || !existing.Active() || existing.SubscriberTenantID != actor.TenantID {
		return Result{}, ErrNotFound
	}
	current, err := s.revisions.Current(ctx, pub.KnowledgeBaseID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if existing.LastSeenRevision >= current {
		return Result{Item: item(existing, pub, current), Changed: false}, nil
	}
	updated, err := s.store.MarkSeen(ctx, existing.ID, current, correlationID, s.now().UTC())
	if err != nil {
		return Result{}, err
	}
	return Result{Item: item(updated, pub, current), Changed: true}, nil
}

func (s *Service) List(ctx context.Context, actor Actor) ([]Item, error) {
	if !validActor(actor) {
		return nil, ErrInvalid
	}
	subscriptions, err := s.store.ListByUser(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]Item, 0, len(subscriptions))
	for _, currentSubscription := range subscriptions {
		if !currentSubscription.Active() || currentSubscription.SubscriberTenantID != actor.TenantID {
			continue
		}
		pub, err := s.pubs.Get(ctx, currentSubscription.PublicationID)
		if errors.Is(err, publication.ErrNotFound) || err == nil && !pub.VisibleTo(actor.TenantID) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		current, err := s.revisions.Current(ctx, pub.KnowledgeBaseID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		result = append(result, item(currentSubscription, pub, current))
	}
	return result, nil
}

func (s *Service) Effective(ctx context.Context, publicationID, userID string, tenantID uint64) (Subscription, error) {
	if publicationID == "" || userID == "" || tenantID == 0 {
		return Subscription{}, ErrInvalid
	}
	result, err := s.store.GetByPublicationUser(ctx, publicationID, userID)
	if err != nil {
		return Subscription{}, err
	}
	if !result.Active() || result.SubscriberTenantID != tenantID {
		return Subscription{}, ErrNotFound
	}
	return result, nil
}

func item(value Subscription, pub publication.Publication, current int64) Item {
	return Item{Subscription: value, Publication: pub, CurrentRevision: current, Updated: current > value.LastSeenRevision}
}

func (s *Service) currentOrPublished(ctx context.Context, pub publication.Publication) (int64, error) {
	current, err := s.revisions.Current(ctx, pub.KnowledgeBaseID)
	if err != nil {
		if pub.PublishedRevision > 0 {
			return pub.PublishedRevision, nil
		}
		return 0, err
	}
	return current, nil
}

func validActor(actor Actor) bool        { return strings.TrimSpace(actor.UserID) != "" && actor.TenantID > 0 }
func validCorrelation(value string) bool { return strings.TrimSpace(value) != "" && len(value) <= 128 }

func (s *Service) recordAudit(ctx context.Context, pub publication.Publication, actor Actor, action string, oldValue, newValue Subscription, correlationID string, at time.Time) error {
	if s.auditor == nil {
		return nil
	}
	var oldJSON, newJSON json.RawMessage
	if oldValue.ID != "" {
		oldJSON, _ = json.Marshal(map[string]any{"status": oldValue.Status, "last_seen_revision": oldValue.LastSeenRevision})
	}
	if newValue.ID != "" {
		newJSON, _ = json.Marshal(map[string]any{"status": newValue.Status, "last_seen_revision": newValue.LastSeenRevision})
	}
	err := s.auditor.Record(ctx, audit.Event{
		TenantID: actor.TenantID, KnowledgeBaseID: pub.KnowledgeBaseID, ActorUserID: actor.UserID,
		Action: action, TargetType: "kb_subscription", TargetID: newValue.ID, Outcome: audit.OutcomeSuccess,
		CorrelationID: correlationID, OldValue: oldJSON, NewValue: newJSON, CreatedAt: at,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func newSubscriptionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
