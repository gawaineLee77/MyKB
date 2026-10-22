package identity

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AdminService struct {
	store Store
	now   func() time.Time
	// DisableKeys invalidates product-owned credentials and advances their
	// creator generation; reactivation must never revive old credentials.
	DisableKeys           func(context.Context, string) error
	RevokeKnowledgeAccess func(context.Context, string, string) error
	// WithStatusLock serializes the complete status and credential-generation
	// transition across gateway instances for a canonical broker subject.
	WithStatusLock func(context.Context, string, func() error) error
}

func NewAdminService(store Store) (*AdminService, error) {
	if store == nil {
		return nil, fmt.Errorf("identity store is required")
	}
	return &AdminService{store: store, now: time.Now}, nil
}

func (s *AdminService) ChangeStatus(ctx context.Context, brokerSubject string, status Status, actorID, correlationID, sourceIP string) (Identity, error) {
	brokerSubject = strings.TrimSpace(brokerSubject)
	if brokerSubject == "" || strings.TrimSpace(actorID) == "" || strings.TrimSpace(correlationID) == "" || (status != StatusActive && status != StatusSuspended) {
		return Identity{}, ErrInvalid
	}
	if s.WithStatusLock == nil {
		if s.DisableKeys != nil || s.RevokeKnowledgeAccess != nil {
			return Identity{}, fmt.Errorf("identity status serialization unavailable")
		}
		return s.changeStatusLocked(ctx, brokerSubject, status, actorID, correlationID, sourceIP)
	}
	var result Identity
	err := s.WithStatusLock(ctx, brokerSubject, func() error {
		var err error
		result, err = s.changeStatusLocked(ctx, brokerSubject, status, actorID, correlationID, sourceIP)
		return err
	})
	return result, err
}

func (s *AdminService) changeStatusLocked(ctx context.Context, brokerSubject string, status Status, actorID, correlationID, sourceIP string) (Identity, error) {
	if (s.DisableKeys != nil || s.RevokeKnowledgeAccess != nil) && status == StatusActive {
		previous, err := s.store.GetByBrokerSubject(ctx, brokerSubject)
		if err != nil {
			return Identity{}, err
		}
		if previous.Status == StatusSuspended && previous.LocalUserID != "" {
			if err := s.revokeAccess(ctx, previous.LocalUserID, actorID); err != nil {
				return Identity{}, err
			}
		}
	}
	updated, err := s.store.SetStatus(ctx, brokerSubject, status, s.now().UTC())
	if err != nil {
		return Identity{}, err
	}
	if status == StatusSuspended && updated.LocalUserID != "" {
		if err := s.revokeAccess(ctx, updated.LocalUserID, actorID); err != nil {
			return Identity{}, err
		}
	}
	action := "identity.activated"
	if status == StatusSuspended {
		action = "identity.suspended"
	}
	id, err := randomUUID()
	if err != nil {
		return Identity{}, err
	}
	if err := s.store.RecordAudit(ctx, AuditEvent{
		ID: id, Issuer: updated.Issuer, Subject: updated.Subject, LocalUserID: actorID,
		Action: action, Outcome: "success", CorrelationID: correlationID, SourceIP: sourceIP,
		CreatedAt: s.now().UTC(),
	}); err != nil {
		return Identity{}, err
	}
	return updated, nil
}

func (s *AdminService) revokeAccess(ctx context.Context, user, actor string) error {
	if s.DisableKeys != nil {
		if err := s.DisableKeys(ctx, user); err != nil {
			return err
		}
	}
	if s.RevokeKnowledgeAccess != nil {
		return s.RevokeKnowledgeAccess(ctx, user, actor)
	}
	return nil
}
