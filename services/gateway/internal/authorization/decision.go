// Package authorization resolves MindCreek KB roles and action permissions.
package authorization

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/grant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

var (
	ErrInvalid     = errors.New("invalid authorization request")
	ErrNotFound    = errors.New("knowledge base not found")
	ErrDenied      = errors.New("knowledge-base access denied")
	ErrUnavailable = errors.New("knowledge-base authorization is unavailable")
)

type Role string

const (
	RoleNone   Role = "none"
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

type Action string

const (
	ActionDiscover     Action = "discover"
	ActionRead         Action = "read"
	ActionEditContent  Action = "edit_content"
	ActionConfigure    Action = "configure"
	ActionManageGrants Action = "manage_grants"
	ActionDelete       Action = "delete"
)

func (a Action) Valid() bool {
	switch a {
	case ActionDiscover, ActionRead, ActionEditContent, ActionConfigure, ActionManageGrants, ActionDelete:
		return true
	default:
		return false
	}
}

type Principal struct {
	UserID   string
	TenantID uint64
}

type Source string

const (
	SourceNone      Source = "none"
	SourceOwner     Source = "owner"
	SourceUserGrant Source = "user_grant"
)

type Decision struct {
	KnowledgeBaseID string
	Role            Role
	Source          Source
	SourceTenantID  uint64
	GrantID         string
	GrantRevision   int64
	ProductMode     profile.ProductMode
}

func (d Decision) Allows(action Action) bool {
	if !action.Valid() {
		return false
	}
	switch d.Role {
	case RoleOwner:
		return true
	case RoleEditor:
		return action == ActionDiscover || action == ActionRead || action == ActionEditContent
	case RoleViewer:
		return action == ActionDiscover || action == ActionRead
	default:
		return false
	}
}

type OwnerResolver interface {
	Resolve(context.Context, string, http.Header) (ownership.Ownership, error)
}

type GrantReader interface {
	EffectiveUserGrant(context.Context, string, string, time.Time) (grant.Grant, error)
}

type Service struct {
	owners OwnerResolver
	grants GrantReader
	now    func() time.Time
}

func NewService(owners OwnerResolver, grants GrantReader, clock func() time.Time) (*Service, error) {
	if owners == nil || grants == nil {
		return nil, fmt.Errorf("owner resolver and grant reader are required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{owners: owners, grants: grants, now: clock}, nil
}

func (s *Service) Decide(ctx context.Context, kbID string, principal Principal, inbound http.Header) (Decision, error) {
	if strings.TrimSpace(kbID) == "" || strings.TrimSpace(principal.UserID) == "" || principal.TenantID == 0 {
		return Decision{}, ErrInvalid
	}
	owner, err := s.owners.Resolve(ctx, kbID, inbound)
	if err != nil {
		if errors.Is(err, ownership.ErrNotFound) || errors.Is(err, ownership.ErrAdoptionRequired) {
			return Decision{}, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return Decision{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	base := Decision{
		KnowledgeBaseID: kbID, Role: RoleNone, Source: SourceNone,
		SourceTenantID: owner.TenantID, ProductMode: owner.ProductMode,
	}
	if principal.UserID == owner.OwnerUserID && principal.TenantID == owner.TenantID {
		base.Role = RoleOwner
		base.Source = SourceOwner
		return base, nil
	}
	if principal.TenantID != owner.TenantID {
		return base, nil
	}
	if owner.IsPersonalNotes() {
		return base, nil
	}
	effective, err := s.grants.EffectiveUserGrant(ctx, kbID, principal.UserID, s.now().UTC())
	if errors.Is(err, grant.ErrNotFound) {
		return base, nil
	}
	if err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	switch effective.Permission {
	case grant.PermissionViewer:
		base.Role = RoleViewer
	case grant.PermissionEditor:
		base.Role = RoleEditor
	default:
		return Decision{}, fmt.Errorf("%w: invalid effective grant permission", ErrUnavailable)
	}
	base.Source = SourceUserGrant
	base.GrantID = effective.ID
	base.GrantRevision = effective.Revision
	return base, nil
}

func (s *Service) Authorize(ctx context.Context, kbID string, principal Principal, action Action, inbound http.Header) (Decision, error) {
	if !action.Valid() {
		return Decision{}, ErrInvalid
	}
	decision, err := s.Decide(ctx, kbID, principal, inbound)
	if err != nil {
		return Decision{}, err
	}
	if !decision.Allows(action) {
		return decision, ErrDenied
	}
	return decision, nil
}
