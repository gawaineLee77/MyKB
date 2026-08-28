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
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
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
	SourceNone               Source = "none"
	SourceOwner              Source = "owner"
	SourceUserGrant          Source = "user_grant"
	SourceSubscription       Source = "subscription"
	SourceOrganizationPublic Source = "organization_public"
)

type Decision struct {
	KnowledgeBaseID string
	Role            Role
	Source          Source
	SourceTenantID  uint64
	GrantID         string
	GrantRevision   int64
	PublicationID   string
	SubscriptionID  string
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

type PublicationReader interface {
	GetPublishedByKB(context.Context, string) (publication.Publication, error)
}

type SubscriptionReader interface {
	Effective(context.Context, string, string, uint64) (subscription.Subscription, error)
}

type Option func(*Service)

func WithPublicationAccess(publications PublicationReader, subscriptions SubscriptionReader) Option {
	return func(service *Service) {
		service.publications = publications
		service.subscriptions = subscriptions
	}
}

type Service struct {
	owners        OwnerResolver
	grants        GrantReader
	publications  PublicationReader
	subscriptions SubscriptionReader
	now           func() time.Time
}

func NewService(owners OwnerResolver, grants GrantReader, clock func() time.Time, options ...Option) (*Service, error) {
	if owners == nil || grants == nil {
		return nil, fmt.Errorf("owner resolver and grant reader are required")
	}
	if clock == nil {
		clock = time.Now
	}
	service := &Service{owners: owners, grants: grants, now: clock}
	for _, option := range options {
		option(service)
	}
	if (service.publications == nil) != (service.subscriptions == nil) {
		return nil, fmt.Errorf("publication and subscription readers must be configured together")
	}
	return service, nil
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
	if owner.IsPersonalNotes() {
		return base, nil
	}
	if principal.TenantID == owner.TenantID {
		effective, err := s.grants.EffectiveUserGrant(ctx, kbID, principal.UserID, s.now().UTC())
		if err == nil {
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
		if !errors.Is(err, grant.ErrNotFound) {
			return Decision{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	if s.publications == nil {
		return base, nil
	}
	pub, err := s.publications.GetPublishedByKB(ctx, kbID)
	if errors.Is(err, publication.ErrNotFound) {
		return base, nil
	}
	if err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !pub.VisibleTo(principal.TenantID) {
		return base, nil
	}
	base.PublicationID = pub.ID
	if pub.AccessMode == publication.AccessOrganizationPublic {
		base.Role = RoleViewer
		base.Source = SourceOrganizationPublic
		return base, nil
	}
	effectiveSubscription, err := s.subscriptions.Effective(ctx, pub.ID, principal.UserID, principal.TenantID)
	if errors.Is(err, subscription.ErrNotFound) {
		return base, nil
	}
	if err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	base.Role = RoleViewer
	base.Source = SourceSubscription
	base.SubscriptionID = effectiveSubscription.ID
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
