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
}

func NewAdminService(store Store) (*AdminService, error) {
	if store == nil {
		return nil, fmt.Errorf("identity store is required")
	}
	return &AdminService{store: store, now: time.Now}, nil
}

func (s *AdminService) ChangeStatus(ctx context.Context, brokerSubject string, status Status, actorID, correlationID, sourceIP string) (Identity, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(correlationID) == "" {
		return Identity{}, ErrInvalid
	}
	updated, err := s.store.SetStatus(ctx, brokerSubject, status, s.now().UTC())
	if err != nil {
		return Identity{}, err
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
