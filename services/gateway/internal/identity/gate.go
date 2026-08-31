package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

var ErrUnlinked = errors.New("session is not linked to an active corporate identity")

// Gate binds the first validated WeKnora principal to its corporate subject
// and rejects later sessions for unlinked or suspended identities.
type Gate struct {
	store         Store
	breakGlassIDs map[string]bool
}

func NewGate(store Store, breakGlassIDs map[string]bool) (*Gate, error) {
	if store == nil {
		return nil, fmt.Errorf("identity store is required")
	}
	return &Gate{store: store, breakGlassIDs: breakGlassIDs}, nil
}

func (g *Gate) Check(ctx context.Context, principal weknora.Principal) error {
	if principal.User == nil || principal.User.ID == "" || principal.Tenant == nil || principal.Tenant.ID == 0 {
		return ErrUnlinked
	}
	if g.breakGlassIDs[strings.ToLower(strings.TrimSpace(principal.User.ID))] {
		return nil
	}
	identity, err := g.store.GetByUpstreamEmail(ctx, principal.User.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnlinked
		}
		return fmt.Errorf("resolve corporate identity: %w", err)
	}
	if identity.Status != StatusActive {
		return ErrSuspended
	}
	if identity.LocalUserID != "" && identity.LocalUserID != principal.User.ID {
		return ErrUnlinked
	}
	if identity.LocalUserID == "" {
		if err := g.store.BindLocalPrincipal(ctx, identity.UpstreamEmail, principal.User.ID, principal.Tenant.ID); err != nil {
			return fmt.Errorf("bind local principal: %w", err)
		}
	}
	return nil
}
