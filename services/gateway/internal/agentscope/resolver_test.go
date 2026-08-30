package agentscope

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/library"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

type libraryStub struct {
	page library.Page
	err  error
}

func (s libraryStub) List(context.Context, library.View, int, int, authorization.Principal, http.Header) (library.Page, error) {
	return s.page, s.err
}

type decisionStub struct {
	decisions map[string]authorization.Decision
	errors    map[string]error
}

func (s decisionStub) Authorize(_ context.Context, id string, _ authorization.Principal, _ authorization.Action, _ http.Header) (authorization.Decision, error) {
	return s.decisions[id], s.errors[id]
}

func TestDefaultScopeContainsOwnedSharedAndSubscribedOnly(t *testing.T) {
	resolver, err := NewResolver(libraryStub{page: library.Page{Total: 4, Items: []library.Item{
		{ID: "owned", Role: authorization.RoleOwner, AccessSource: authorization.SourceOwner, ProductMode: profile.ModeRAG},
		{ID: "shared", Role: authorization.RoleViewer, AccessSource: authorization.SourceUserGrant, ProductMode: profile.ModeRAG},
		{ID: "subscribed", Role: authorization.RoleViewer, AccessSource: authorization.SourceSubscription, ProductMode: profile.ModeRAG},
		{ID: "public", Role: authorization.RoleViewer, AccessSource: authorization.SourceOrganizationPublic, ProductMode: profile.ModeRAG},
	}}}, decisionStub{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.Resolve(context.Background(), Request{Selection: SelectionDefault}, authorization.Principal{UserID: "alice", TenantID: 42}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"owned", "shared", "subscribed"}
	if len(got.KnowledgeBaseIDs) != len(want) {
		t.Fatalf("scope IDs = %v, want %v", got.KnowledgeBaseIDs, want)
	}
	for index := range want {
		if got.KnowledgeBaseIDs[index] != want[index] {
			t.Fatalf("scope IDs = %v, want %v", got.KnowledgeBaseIDs, want)
		}
	}
}

func TestExplicitScopeAllowsSelectedOrganizationPublic(t *testing.T) {
	decision := authorization.Decision{KnowledgeBaseID: "public", Role: authorization.RoleViewer, Source: authorization.SourceOrganizationPublic, ProductMode: profile.ModeRAG}
	resolver, _ := NewResolver(libraryStub{}, decisionStub{decisions: map[string]authorization.Decision{"public": decision}, errors: map[string]error{}})
	got, err := resolver.Resolve(context.Background(), Request{Selection: SelectionExplicit, KnowledgeBaseIDs: []string{"public"}}, authorization.Principal{UserID: "alice", TenantID: 42}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].AccessSource != authorization.SourceOrganizationPublic {
		t.Fatalf("unexpected explicit result: %+v", got)
	}
}

func TestExplicitScopePreservesEveryReadableAccessSource(t *testing.T) {
	decisions := map[string]authorization.Decision{
		"editor":     {KnowledgeBaseID: "editor", Role: authorization.RoleEditor, Source: authorization.SourceUserGrant, ProductMode: profile.ModeRAG},
		"notes":      {KnowledgeBaseID: "notes", Role: authorization.RoleOwner, Source: authorization.SourceOwner, ProductMode: profile.ModePersonalNotes},
		"owner":      {KnowledgeBaseID: "owner", Role: authorization.RoleOwner, Source: authorization.SourceOwner, ProductMode: profile.ModeRAG},
		"public":     {KnowledgeBaseID: "public", Role: authorization.RoleViewer, Source: authorization.SourceOrganizationPublic, ProductMode: profile.ModeRAG},
		"subscriber": {KnowledgeBaseID: "subscriber", Role: authorization.RoleViewer, Source: authorization.SourceSubscription, ProductMode: profile.ModeRAG},
		"viewer":     {KnowledgeBaseID: "viewer", Role: authorization.RoleViewer, Source: authorization.SourceUserGrant, ProductMode: profile.ModeRAG},
	}
	resolver, _ := NewResolver(libraryStub{}, decisionStub{decisions: decisions, errors: map[string]error{}})
	got, err := resolver.Resolve(context.Background(), Request{Selection: SelectionExplicit, KnowledgeBaseIDs: []string{
		"viewer", "owner", "public", "subscriber", "editor", "notes",
	}}, authorization.Principal{UserID: "alice", TenantID: 42}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != len(decisions) {
		t.Fatalf("entries = %+v", got.Entries)
	}
	for index := 1; index < len(got.KnowledgeBaseIDs); index++ {
		if got.KnowledgeBaseIDs[index-1] >= got.KnowledgeBaseIDs[index] {
			t.Fatalf("scope is not deterministic: %v", got.KnowledgeBaseIDs)
		}
	}
	if got.Entries[1].KnowledgeBaseID != "notes" || got.Entries[1].ProductMode != string(profile.ModePersonalNotes) {
		t.Fatalf("owner-only note entry was not preserved: %+v", got.Entries)
	}
}

func TestRevokedUnpublishedAndWrongWorkspaceScopesAreNonDisclosing(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "revoked grant", err: authorization.ErrDenied},
		{name: "inactive subscription", err: authorization.ErrDenied},
		{name: "unpublished knowledge", err: authorization.ErrNotFound},
		{name: "wrong workspace", err: authorization.ErrDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver, _ := NewResolver(libraryStub{}, decisionStub{decisions: map[string]authorization.Decision{}, errors: map[string]error{"target": test.err}})
			_, err := resolver.Resolve(context.Background(), Request{Selection: SelectionExplicit, KnowledgeBaseIDs: []string{"target"}}, authorization.Principal{UserID: "alice", TenantID: 42}, nil)
			if !errors.Is(err, ErrDenied) || strings.Contains(err.Error(), "target") {
				t.Fatalf("non-disclosing denial = %v", err)
			}
		})
	}
}

func TestExplicitScopeFailsClosedWithoutDisclosingWhichID(t *testing.T) {
	resolver, _ := NewResolver(libraryStub{}, decisionStub{
		decisions: map[string]authorization.Decision{"owned": {KnowledgeBaseID: "owned", Role: authorization.RoleOwner}},
		errors:    map[string]error{"private": authorization.ErrDenied},
	})
	_, err := resolver.Resolve(context.Background(), Request{Selection: SelectionExplicit, KnowledgeBaseIDs: []string{"owned", "private"}}, authorization.Principal{UserID: "alice", TenantID: 42}, nil)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("error = %v, want denied", err)
	}
}

func TestScopeRejectsInvalidAndOversizedRequests(t *testing.T) {
	resolver, _ := NewResolver(libraryStub{}, decisionStub{})
	principal := authorization.Principal{UserID: "alice", TenantID: 42}
	if _, err := resolver.Resolve(context.Background(), Request{Selection: SelectionExplicit}, principal, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty explicit error = %v", err)
	}
	ids := make([]string, MaxKnowledgeBases+1)
	for index := range ids {
		ids[index] = "kb"
	}
	if _, err := resolver.Resolve(context.Background(), Request{Selection: SelectionExplicit, KnowledgeBaseIDs: ids}, principal, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized error = %v", err)
	}
}
