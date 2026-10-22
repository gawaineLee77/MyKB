package resourceacl

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
)

type fakeCatalog struct {
	resource Resource
	err      error
	uploaded bool
}

func (f *fakeCatalog) Resolve(context.Context, uint64, string) (Resource, error) {
	return f.resource, f.err
}
func (f *fakeCatalog) MessageImage(context.Context, uint64, string, string, string) (bool, error) {
	return f.uploaded, nil
}

type fakeAccess struct {
	tenant  uint64
	manager bool
	visible map[string]bool
}

func (f *fakeAccess) Authorize(_ context.Context, _ http.Header, kb string, _ ...string) (kbacl.Permissions, error) {
	if !f.visible[kb] {
		return kbacl.Permissions{}, &kbacl.Error{Status: 404, Code: "kb_acl.not_found"}
	}
	// A creator's manage privilege must not become a workspace-wide bypass.
	return kbacl.Permissions{Visible: true, Manage: true, Creator: true}, nil
}
func (f *fakeAccess) ResourceIdentity(context.Context, http.Header) (uint64, bool, error) {
	return f.tenant, f.manager, nil
}
func assertStatus(t *testing.T, err error, status int) {
	t.Helper()
	if status == 0 {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var public interface{ HTTPStatus() int }
	if !errors.As(err, &public) || public.HTTPStatus() != status {
		t.Fatalf("want %d, got %v", status, err)
	}
}
func TestResourceAccessRequiresActualProvenanceAndFreshACL(t *testing.T) {
	ctx := context.Background()
	access := &fakeAccess{tenant: 7, visible: map[string]bool{"visible": true}}
	catalog := &fakeCatalog{resource: Resource{TenantID: 7, Owners: []Owner{{KBID: "hidden", KnowledgeID: "doc"}}}}
	s := &Service{Catalog: catalog, Access: access}
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 404)
	catalog.resource.Owners[0].KBID = "visible"
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 0)
	access.visible["visible"] = false
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 404)
	access.visible["visible"] = true
	catalog.resource.TenantID = 8
	access.manager = true
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 404)
	catalog.resource.TenantID = 7
	catalog.resource.Owners = nil
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 0)
	access.manager = false
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 409)
	catalog.err = errors.New("synthetic storage outage")
	assertStatus(t, s.AuthorizeKB(ctx, nil, "visible", "resource://synthetic"), 503)
}
func TestMessageImageScopeAndUploads(t *testing.T) {
	ctx := context.Background()
	access := &fakeAccess{tenant: 7, visible: map[string]bool{"a": true, "b": true}}
	catalog := &fakeCatalog{resource: Resource{TenantID: 7, Owners: []Owner{{KBID: "b", KnowledgeID: "doc"}}}}
	s := &Service{Catalog: catalog, Access: access}
	check := func(kbs []string, status int) {
		t.Helper()
		assertStatus(t, s.AuthorizeMessage(ctx, nil, "session", "message", kbs, "resource://synthetic"), status)
	}
	check([]string{"a"}, 404)
	check([]string{"b"}, 0)
	access.visible["b"] = false
	check([]string{"b"}, 404)
	catalog.resource.Owners = nil
	check(nil, 409)
	catalog.uploaded = true
	check(nil, 0)
	catalog.resource.TenantID = 8
	check(nil, 404)
}

func TestWorkspaceManagerCannotReadAnotherUsersPrivateChatUpload(t *testing.T) {
	ctx := context.Background()
	access := &fakeAccess{tenant: 7, manager: true, visible: map[string]bool{"kb": true}}
	catalog := &fakeCatalog{resource: Resource{TenantID: 7, SessionScoped: true}}
	s := &Service{Catalog: catalog, Access: access}
	assertStatus(t, s.AuthorizeKB(ctx, nil, "kb", "resource://synthetic"), 404)
	assertStatus(t, s.AuthorizeMessage(ctx, nil, "own-session", "own-message", nil, "resource://synthetic"), 404)
	catalog.uploaded = true
	assertStatus(t, s.AuthorizeMessage(ctx, nil, "own-session", "own-message", nil, "resource://synthetic"), 0)
	// Independently verified saving into a KB can give a resource both owners.
	catalog.resource.Owners = []Owner{{KBID: "kb", KnowledgeID: "doc"}}
	assertStatus(t, s.AuthorizeKB(ctx, nil, "kb", "resource://synthetic"), 0)
}
