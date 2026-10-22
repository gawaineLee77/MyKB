package database_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/branding"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/database"
)

// Existing migration tests target their own schema version regardless of
// subsequently added product migrations. Only disposable test DBs call this.
func rollbackToVersion(t *testing.T, ctx context.Context, r *database.Runner, target int64) {
	t.Helper()
	status, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	steps := 0
	for _, m := range status {
		if m.Version > target {
			steps++
		}
	}
	if steps > 0 {
		if err := r.Down(ctx, steps); err != nil {
			t.Fatal("isolate migration", target, err)
		}
	}
}

func TestBrandingMigrationAndRepository(t *testing.T) {
	dsn := os.Getenv("MINDCREEK_PERMISSION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set a disposable permission_test database DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/permission_test" {
		t.Fatal("requires loopback synthetic permission_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner, err := database.NewRunner(db)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err := runner.Up(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rollbackToVersion(t, ctx, runner, 20)
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("virgin rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("forward upgrade", err)
	}
	rollbackToVersion(t, ctx, runner, 20)
	r := &branding.Repository{DB: db}
	initial, err := r.Get(ctx)
	if err != nil || initial.Name != branding.DefaultName || initial.Revision != 1 {
		t.Fatal(initial, err)
	}
	updated, err := r.Update(ctx, branding.Change{Name: "合成测试名称", Revision: 1, ReplaceLogo: true, Logo: []byte("synthetic image bytes")}, "synthetic-installed-admin", "synthetic-request")
	if err != nil || updated.Revision != 2 || len(updated.Logo) == 0 {
		t.Fatal(updated, err)
	}
	if _, err := r.Update(ctx, branding.Change{Name: "stale", Revision: 1}, "synthetic", ""); !errors.Is(err, branding.ErrConflict) {
		t.Fatal("optimistic concurrency", err)
	}
	var count int
	var actor, before, after string
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mindcreek.service_branding_events`).Scan(&count); err != nil || count != 1 {
		t.Fatal("atomic audit", count, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT actor_user_id,before_state->>'name',after_state->>'name' FROM mindcreek.service_branding_events`).Scan(&actor, &before, &after); err != nil || actor != "synthetic-installed-admin" || before != branding.DefaultName || after != updated.Name {
		t.Fatal("audit actor/state", err)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback erased brand configuration")
	}
	if got, err := r.Get(ctx); err != nil || got.Name != updated.Name {
		t.Fatal("blocked rollback changed configuration", err)
	}
	// A failing audit insert must also roll back the settings update.
	if _, err := db.ExecContext(ctx, `ALTER TABLE mindcreek.service_branding_events ADD CONSTRAINT synthetic_audit_failure CHECK (actor_user_id <> 'synthetic-denied')`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Update(ctx, branding.Change{Name: "must roll back", Revision: 2}, "synthetic-denied", ""); err == nil {
		t.Fatal("audit failure accepted")
	}
	if got, err := r.Get(ctx); err != nil || got.Revision != 2 {
		t.Fatal("non-atomic audit", err)
	}
	// Guarded synthetic-only cleanup; configured production rows are preserved.
	if _, err := db.ExecContext(ctx, `DELETE FROM mindcreek.service_branding_events; UPDATE mindcreek.service_branding SET name='小纳',logo=''::bytea,revision=1,updated_by=''; ALTER TABLE mindcreek.service_branding_events DROP CONSTRAINT synthetic_audit_failure`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("empty rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("final forward upgrade", err)
	}
}
