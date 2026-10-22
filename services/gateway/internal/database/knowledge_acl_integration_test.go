package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/database"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/kbacl"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
)

func TestKnowledgeACLMigrationsAndRevocation(t *testing.T) {
	dsn := os.Getenv("MINDCREEK_PERMISSION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set a disposable permission_test database DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/permission_test" {
		t.Fatal("requires loopback synthetic permission_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lockDB, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer lockDB.Close()
	runner, err := database.NewRunner(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := runner.Up(ctx); err != nil {
			t.Fatal(err)
		}
	}
	status, err := runner.Status(ctx)
	if err != nil || len(status) < 19 {
		t.Fatal(status, err)
	}
	rollbackToVersion(t, ctx, runner, 19)
	if err := runner.Down(ctx, 2); err != nil {
		t.Fatal("empty ACL rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("ACL forward upgrade", err)
	}
	rollbackToVersion(t, ctx, runner, 19)
	store := kbacl.NewRepository(db)
	p, err := store.Ensure(kbacl.WithAuditActor(ctx, "synthetic-owner"), kbacl.Policy{TenantID: 42, KBID: "synthetic-kb", CreatorUserID: "synthetic-creator"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Visibility != "restricted" || p.Revision != 1 {
		t.Fatal("migration widened default", p)
	}
	if _, err := store.Policy(ctx, 43, p.KBID); !errors.Is(err, kbacl.ErrNotFound) {
		t.Fatal("cross-space policy", err)
	}
	p.Members = []kbacl.Member{{UserID: "synthetic-collaborator", Visible: true, Operations: []string{"document.create", "document.download"}}}
	e := kbacl.Event{TenantID: p.TenantID, KBID: p.KBID, ActorUserID: "synthetic-creator", Action: "policy.update", Outcome: "success", Before: json.RawMessage(`{}`)}
	p, err = store.Save(ctx, p, 1, e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(ctx, p, 1, e); !errors.Is(err, kbacl.ErrConflict) {
		t.Fatal("stale policy overwrote grants", err)
	}
	if err := store.RevokeMember(kbacl.WithAuditActor(ctx, "synthetic-admin"), 42, "synthetic-collaborator", false); err != nil {
		t.Fatal(err)
	}
	p, err = store.Policy(ctx, 42, p.KBID)
	if err != nil || len(p.Members) != 1 || len(p.Members[0].Operations) != 0 || !p.Members[0].Visible {
		t.Fatal("downgrade did not retain view-only", p, err)
	}
	if err := store.RevokeMember(kbacl.WithAuditActor(ctx, "synthetic-admin"), 42, "synthetic-creator", true); err != nil {
		t.Fatal(err)
	}
	p, err = store.Policy(ctx, 42, p.KBID)
	if err != nil || !p.CreatorRevoked {
		t.Fatal("creator rejoin could restore authority", p, err)
	}
	if err := store.RevokeIdentity(kbacl.WithAuditActor(ctx, "synthetic-platform-admin"), "synthetic-collaborator"); err != nil {
		t.Fatal(err)
	}
	g, err := store.Generation(ctx, 42, "synthetic-collaborator")
	if err != nil || g.Identity != 1 {
		t.Fatal("identity generation not durable", g, err)
	}
	events, err := store.Events(ctx, 42, p.KBID)
	if err != nil || len(events) < 4 || events[0].ActorUserID != "synthetic-platform-admin" {
		t.Fatal("audit attribution", events, err)
	}
	// An ambiguous native creation cannot be mistaken for a historical KB.
	op := kbacl.CreateOperation{ID: "synthetic-create", TenantID: 42, ActorUserID: "synthetic-creator", KeyHash: strings.Repeat("a", 64), RequestHash: strings.Repeat("b", 64), BaselineIDs: []string{"historical-kb"}}
	created, fresh, err := store.BeginCreation(ctx, op)
	if err != nil || !fresh || len(created.BaselineIDs) != 1 {
		t.Fatal("creation baseline did not round-trip", created, err)
	}
	if _, fresh, err := store.BeginCreation(ctx, op); err != nil || fresh {
		t.Fatal("duplicate creation admitted", err)
	}
	for _, tc := range []struct {
		tenant      uint64
		creator, kb string
		pending     bool
	}{
		{42, "synthetic-creator", "historical-kb", false},
		{42, "synthetic-creator", p.KBID, true},
		{43, "synthetic-creator", p.KBID, false},
		{42, "different-creator", p.KBID, false},
	} {
		pending, err := store.CreationPending(ctx, tc.tenant, tc.creator, tc.kb)
		if err != nil || pending != tc.pending {
			t.Fatalf("pending boundary %+v: %v %v", tc, pending, err)
		}
	}
	if err := store.CompleteCreation(kbacl.WithAuditActor(ctx, "synthetic-admin"), 42, op.ID, p.KBID); err != nil {
		t.Fatal("reconcile creation", err)
	}
	if pending, err := store.CreationPending(ctx, 42, op.ActorUserID, p.KBID); err != nil || pending {
		t.Fatal("reconciled KB still quarantined", err)
	}
	execution := &workspaceadmin.KnowledgeRepository{DB: db}
	policy := workspaceadmin.MachinePolicy{TenantID: 42, KeyID: 9, Fingerprint: strings.Repeat("e", 64), KnowledgeBaseIDs: []string{p.KBID}, Operations: []string{"chunk.export"}}
	if err := execution.SaveMachinePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback lost live key policy")
	}
	if _, err := execution.MachinePolicy(ctx, 43, 9); !errors.Is(err, workspaceadmin.ErrNotFound) {
		t.Fatal("cross-space machine policy", err)
	}
	if err := execution.DeleteMachinePolicy(ctx, 42, 9); err != nil {
		t.Fatal(err)
	}
	if err := execution.SaveExecutionDelegation(ctx, workspaceadmin.Delegation{TenantID: 42, OwnerUserID: "synthetic-owner", SessionCipher: []byte("encrypted-synthetic-placeholder")}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback lost live content delegation")
	}
	if err := execution.DeleteExecutionDelegation(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback lost ACL tombstones")
	}
	// Exercise the complete ordered chain with fewer connections than callers.
	db.SetMaxOpenConns(1)
	lockDB.SetMaxOpenConns(2)
	locks := workspaceadmin.NewRepository(db, lockDB)
	done := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func(i int) {
			done <- locks.WithLocks(ctx, []string{fmt.Sprintf("identity-user:synthetic-%d", i%3), "workspace-admin:42", "kb-acl:42:synthetic-kb"}, func(workspaceadmin.Store) error {
				_, err := store.Generation(ctx, 42, "synthetic-collaborator")
				return err
			})
		}(i)
	}
	for i := 0; i < 12; i++ {
		if err := <-done; err != nil {
			t.Fatal("ordered lock pool starvation", err)
		}
	}
	// This cleanup is only in the guarded disposable synthetic database.
	for _, table := range []string{"kb_creation_operations", "kb_acl_events", "kb_member_permissions", "kb_policies", "kb_acl_generations"} {
		if _, err := db.ExecContext(ctx, "DELETE FROM mindcreek."+table); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("empty rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("re-upgrade", err)
	}
}
