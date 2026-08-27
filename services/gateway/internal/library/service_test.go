package library

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type upstreamStub struct {
	items []weknora.KnowledgeBase
	err   error
}

func (s upstreamStub) ListKnowledgeBases(context.Context, http.Header) ([]weknora.KnowledgeBase, error) {
	return s.items, s.err
}

type decisionStub struct {
	items map[string]authorization.Decision
	errs  map[string]error
}

func (s decisionStub) Decide(_ context.Context, kbID string, _ authorization.Principal, _ http.Header) (authorization.Decision, error) {
	return s.items[kbID], s.errs[kbID]
}

func TestListSeparatesOwnedAndShared(t *testing.T) {
	service, err := NewService(upstreamStub{items: []weknora.KnowledgeBase{
		{ID: "owned", Name: "Owned", TenantID: 42, CreatorID: "alice"},
		{ID: "viewer", Name: "Viewer", TenantID: 42, CreatorID: "bob"},
		{ID: "editor", Name: "Editor", TenantID: 42, CreatorID: "carol"},
		{ID: "peer", Name: "Peer", TenantID: 42, CreatorID: "dave"},
	}}, decisionStub{items: map[string]authorization.Decision{
		"owned":  {Role: authorization.RoleOwner, ProductMode: profile.ModeRAG},
		"viewer": {Role: authorization.RoleViewer},
		"editor": {Role: authorization.RoleEditor},
		"peer":   {Role: authorization.RoleNone},
	}, errs: map[string]error{}})
	if err != nil {
		t.Fatal(err)
	}
	principal := authorization.Principal{UserID: "alice", TenantID: 42}
	owned, err := service.List(context.Background(), ViewOwned, 1, 20, principal, nil)
	if err != nil || owned.Total != 1 || owned.Items[0].ID != "owned" || owned.Items[0].ProductMode != profile.ModeRAG {
		t.Fatalf("owned = %+v, %v", owned, err)
	}
	shared, err := service.List(context.Background(), ViewShared, 1, 20, principal, nil)
	if err != nil || shared.Total != 2 || shared.Items[0].ID != "viewer" || shared.Items[1].ID != "editor" {
		t.Fatalf("shared = %+v, %v", shared, err)
	}
}

func TestListFailsClosedOnDecisionDependency(t *testing.T) {
	service, _ := NewService(upstreamStub{items: []weknora.KnowledgeBase{{ID: "kb-1", TenantID: 42}}}, decisionStub{
		items: map[string]authorization.Decision{}, errs: map[string]error{"kb-1": authorization.ErrUnavailable},
	})
	_, err := service.List(context.Background(), ViewOwned, 1, 20, authorization.Principal{UserID: "alice", TenantID: 42}, nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List() error = %v", err)
	}
}
