package authorization

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/grant"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/ownership"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/subscription"
)

type ownerStub struct {
	result ownership.Ownership
	err    error
}

func (s ownerStub) Resolve(context.Context, string, http.Header) (ownership.Ownership, error) {
	return s.result, s.err
}

type grantReaderStub struct {
	result grant.Grant
	err    error
	calls  int
}

func (s *grantReaderStub) EffectiveUserGrant(context.Context, string, string, time.Time) (grant.Grant, error) {
	s.calls++
	return s.result, s.err
}

func TestDecisionRolesAndActionMatrix(t *testing.T) {
	now := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	owner := ownership.Ownership{KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModeRAG}
	actions := []Action{ActionDiscover, ActionRead, ActionEditContent, ActionConfigure, ActionManageGrants, ActionDelete}
	tests := []struct {
		name       string
		principal  Principal
		grant      grant.Grant
		grantErr   error
		wantRole   Role
		wantAllows map[Action]bool
	}{
		{
			name: "owner", principal: Principal{UserID: "alice", TenantID: 42}, wantRole: RoleOwner,
			wantAllows: map[Action]bool{ActionDiscover: true, ActionRead: true, ActionEditContent: true, ActionConfigure: true, ActionManageGrants: true, ActionDelete: true},
		},
		{
			name: "viewer", principal: Principal{UserID: "bob", TenantID: 42},
			grant: grant.Grant{ID: "g-view", Permission: grant.PermissionViewer, Revision: 1}, wantRole: RoleViewer,
			wantAllows: map[Action]bool{ActionDiscover: true, ActionRead: true},
		},
		{
			name: "editor", principal: Principal{UserID: "carol", TenantID: 42},
			grant: grant.Grant{ID: "g-edit", Permission: grant.PermissionEditor, Revision: 4}, wantRole: RoleEditor,
			wantAllows: map[Action]bool{ActionDiscover: true, ActionRead: true, ActionEditContent: true},
		},
		{
			name: "peer", principal: Principal{UserID: "dave", TenantID: 42}, grantErr: grant.ErrNotFound, wantRole: RoleNone,
			wantAllows: map[Action]bool{},
		},
		{
			name: "expired grantee", principal: Principal{UserID: "erin", TenantID: 42}, grantErr: grant.ErrNotFound, wantRole: RoleNone,
			wantAllows: map[Action]bool{},
		},
		{
			name: "revoked grantee", principal: Principal{UserID: "frank", TenantID: 42}, grantErr: grant.ErrNotFound, wantRole: RoleNone,
			wantAllows: map[Action]bool{},
		},
		{
			name: "owner id in wrong tenant", principal: Principal{UserID: "alice", TenantID: 99},
			grant: grant.Grant{ID: "must-not-apply", Permission: grant.PermissionEditor, Revision: 1}, wantRole: RoleNone,
			wantAllows: map[Action]bool{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &grantReaderStub{result: test.grant, err: test.grantErr}
			service := mustAuthorizationService(t, ownerStub{result: owner}, reader, now)
			decision, err := service.Decide(context.Background(), "kb-1", test.principal, nil)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Role != test.wantRole {
				t.Fatalf("Decide() role = %q, want %q", decision.Role, test.wantRole)
			}
			for _, action := range actions {
				if decision.Allows(action) != test.wantAllows[action] {
					t.Errorf("role %q Allows(%q) = %t, want %t", decision.Role, action, decision.Allows(action), test.wantAllows[action])
				}
			}
			if test.wantRole == RoleOwner && reader.calls != 0 {
				t.Fatal("owner decision queried grants")
			}
			if test.principal.TenantID != owner.TenantID && reader.calls != 0 {
				t.Fatal("wrong-tenant decision queried grants")
			}
		})
	}
}

func TestPersonalNotesIgnoreStrayGrant(t *testing.T) {
	now := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	reader := &grantReaderStub{result: grant.Grant{ID: "unexpected", Permission: grant.PermissionEditor}}
	service := mustAuthorizationService(t, ownerStub{result: ownership.Ownership{
		KnowledgeBaseID: "notes-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModePersonalNotes,
	}}, reader, now)
	decision, err := service.Decide(context.Background(), "notes-1", Principal{UserID: "bob", TenantID: 42}, nil)
	if err != nil || decision.Role != RoleNone || reader.calls != 0 {
		t.Fatalf("Personal Notes decision = %+v, %v; grant calls=%d", decision, err, reader.calls)
	}
}

func TestAuthorizeDeniesWithoutRequiredRole(t *testing.T) {
	now := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	service := mustAuthorizationService(t, ownerStub{result: ownership.Ownership{
		KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModeRAG,
	}}, &grantReaderStub{result: grant.Grant{ID: "grant-1", Permission: grant.PermissionViewer, Revision: 1}}, now)
	decision, err := service.Authorize(context.Background(), "kb-1", Principal{UserID: "bob", TenantID: 42}, ActionEditContent, nil)
	if !errors.Is(err, ErrDenied) || decision.Role != RoleViewer {
		t.Fatalf("Authorize() = %+v, %v", decision, err)
	}
}

func TestDecisionFailsClosedWhenDependenciesFail(t *testing.T) {
	now := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		owners ownerStub
		grants *grantReaderStub
	}{
		{name: "ownership", owners: ownerStub{err: ownership.ErrConflict}, grants: &grantReaderStub{}},
		{name: "grant store", owners: ownerStub{result: ownership.Ownership{KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42}}, grants: &grantReaderStub{err: errors.New("database offline")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := mustAuthorizationService(t, test.owners, test.grants, now)
			_, err := service.Decide(context.Background(), "kb-1", Principal{UserID: "bob", TenantID: 42}, nil)
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Decide() error = %v", err)
			}
		})
	}
}

type publicationReaderStub struct {
	result publication.Publication
	err    error
}

func (s publicationReaderStub) GetPublishedByKB(context.Context, string) (publication.Publication, error) {
	return s.result, s.err
}

type subscriptionReaderStub struct {
	result subscription.Subscription
	err    error
}

func (s subscriptionReaderStub) Effective(context.Context, string, string, uint64) (subscription.Subscription, error) {
	return s.result, s.err
}

func TestPublicationDerivedReadRolesNeverElevateEdits(t *testing.T) {
	now := time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC)
	owner := ownership.Ownership{KnowledgeBaseID: "kb-1", OwnerUserID: "alice", TenantID: 42, ProductMode: profile.ModeRAG}
	basePublication := publication.Publication{
		ID: "pub-1", KnowledgeBaseID: "kb-1", PublisherID: "alice", PublisherTenantID: 42,
		Title: "Guide", Audience: publication.Audience{Type: publication.AudienceOrganization},
		Status: publication.StatusPublished, PublishedRevision: 2, CreatedAt: now, PublishedAt: now,
		UpdatedAt: now, RowVersion: 1, LastAuditCorrelationID: "request-0",
	}
	tests := []struct {
		name         string
		mode         publication.AccessMode
		subscription subscriptionReaderStub
		wantSource   Source
	}{
		{name: "organization public", mode: publication.AccessOrganizationPublic, wantSource: SourceOrganizationPublic},
		{name: "subscriber", mode: publication.AccessSubscriber, subscription: subscriptionReaderStub{result: subscription.Subscription{ID: "sub-1", Status: subscription.StatusActive}}, wantSource: SourceSubscription},
		{name: "not subscribed", mode: publication.AccessSubscriber, subscription: subscriptionReaderStub{err: subscription.ErrNotFound}, wantSource: SourceNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pub := basePublication
			pub.AccessMode = test.mode
			service, err := NewService(ownerStub{result: owner}, &grantReaderStub{err: grant.ErrNotFound}, func() time.Time { return now },
				WithPublicationAccess(publicationReaderStub{result: pub}, test.subscription))
			if err != nil {
				t.Fatal(err)
			}
			decision, err := service.Decide(context.Background(), "kb-1", Principal{UserID: "bob", TenantID: 99}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Source != test.wantSource {
				t.Fatalf("source = %q, want %q", decision.Source, test.wantSource)
			}
			if test.wantSource != SourceNone && (!decision.Allows(ActionRead) || decision.Allows(ActionEditContent)) {
				t.Fatalf("publication decision permissions = %+v", decision)
			}
		})
	}
}

func mustAuthorizationService(t *testing.T, owners OwnerResolver, grants GrantReader, now time.Time) *Service {
	t.Helper()
	service, err := NewService(owners, grants, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return service
}
