package database_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/database"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/workspaceadmin"
)

// The opt-in DSN must point to a disposable, synthetic-only database.
func TestWorkspaceAdminMigrationAndRepository(t *testing.T) {
	dsn := os.Getenv("MINDCREEK_PERMISSION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set a disposable permission_test database DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Path != "/permission_test" || u.Hostname() != "127.0.0.1" {
		t.Fatal("permission test requires loopback permission_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
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
	if err := runner.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("repeat upgrade", err)
	}
	// Test migration 17 in isolation; later ACL migrations have independent
	// populated-data rollback guards exercised in knowledge_acl_integration_test.
	rollbackToVersion(t, ctx, runner, 17)
	status, err := runner.Status(ctx)
	if err != nil || len(status) != 17 || status[0].Version != 17 {
		t.Fatal("migration status", status, err)
	}
	store := workspaceadmin.NewRepository(db, lockDB)
	k := workspaceadmin.KeyOwner{TenantID: 42, KeyID: 7, CreatorUserID: "synthetic-admin", RequiredRole: "admin", Fingerprint: strings.Repeat("a", 64)}
	if err := store.SaveKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Key(ctx, 43, 7); err != workspaceadmin.ErrNotFound {
		t.Fatal("cross-space binding", err)
	}
	if err := store.DisableUserKeys(ctx, 0, k.CreatorUserID); err != nil {
		t.Fatal(err)
	}
	saved, err := store.KeyByFingerprint(ctx, k.Fingerprint)
	if err != nil || !saved.Disabled {
		t.Fatal("revocation did not persist", err)
	}
	gen, err := store.CreatorGeneration(ctx, k.CreatorUserID)
	if err != nil || gen != 1 {
		t.Fatal("generation", gen, err)
	}
	k.KeyID = 8
	k.Fingerprint = strings.Repeat("b", 64)
	if err := store.SaveKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	late, err := store.Key(ctx, 42, 8)
	if err != nil || late.CreatorGeneration == gen {
		t.Fatal("late pre-suspension create lost its generation")
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback erased live revocation metadata")
	}
	if _, err := store.Key(ctx, 42, 7); err != nil {
		t.Fatal("failed rollback changed metadata", err)
	}
	// Exercise real shared locks: a cancelled concurrent request must not enter.
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.WithLock(ctx, "synthetic-shared-lock", func(workspaceadmin.Store) error { close(entered); <-release; return nil })
	}()
	<-entered
	short, stop := context.WithTimeout(ctx, 80*time.Millisecond)
	err = store.WithLock(short, "synthetic-shared-lock", func(workspaceadmin.Store) error { t.Error("concurrent lock admitted"); return nil })
	stop()
	close(release)
	if err == nil {
		t.Fatal("lock ignored cancellation")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Saturate a tiny lock pool while lock holders issue product callbacks on
	// the normal query pool. A shared pool would deadlock on these lookups.
	db.SetMaxOpenConns(1)
	lockDB.SetMaxOpenConns(2)
	concurrent, cancelConcurrent := context.WithTimeout(ctx, 5*time.Second)
	defer cancelConcurrent()
	completed := make(chan error, 12)
	for n := 0; n < 12; n++ {
		go func(n int) {
			completed <- store.WithLock(concurrent, fmt.Sprintf("synthetic-pool-%d", n%3), func(locked workspaceadmin.Store) error {
				if _, err := locked.CreatorGeneration(concurrent, k.CreatorUserID); err != nil {
					return err
				}
				var one int
				return db.QueryRowContext(concurrent, "SELECT 1").Scan(&one)
			})
		}(n)
	}
	for n := 0; n < 12; n++ {
		if err := <-completed; err != nil {
			t.Fatal("lock waiters starved the query pool", err)
		}
	}
	// Synthetic-only cleanup permits testing an unused-schema rollback. Real
	// deployments retain these tables; an older gateway also requires revoking
	// managed native keys before losing its runtime enforcement.
	if _, err := db.ExecContext(ctx, `DELETE FROM mindcreek.workspace_api_key_owners; DELETE FROM mindcreek.workspace_api_key_creator_generations`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("empty rollback", err)
	}
	status, err = runner.Status(ctx)
	if err != nil || len(status) != 16 {
		t.Fatal("rollback status", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("forward upgrade", err)
	}
	rollbackToVersion(t, ctx, runner, 17)
	if err := store.SaveDelegation(ctx, workspaceadmin.Delegation{TenantID: 42, OwnerUserID: "synthetic-owner", SessionCipher: []byte("synthetic-encrypted-placeholder")}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback erased a delegation")
	}
	if err := store.DeleteDelegation(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePending(ctx, 42, "synthetic-pending", 0); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback erased unknown create outcome")
	}
	if err := store.DeletePending(ctx, 42, "synthetic-pending"); err != nil {
		t.Fatal(err)
	}
}
