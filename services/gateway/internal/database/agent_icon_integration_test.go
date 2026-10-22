package database_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/agenticon"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/database"
)

func TestAgentIconMigrationRepositoryAndConcurrentQuota(t *testing.T) {
	dsn := os.Getenv("MINDCREEK_PERMISSION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set a disposable permission_test database DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/permission_test" {
		t.Fatal("requires loopback synthetic permission_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
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
	for i := 0; i < 2; i++ {
		if err := runner.Up(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rollbackToVersion(t, ctx, runner, 21)
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("empty rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("forward upgrade", err)
	}
	r := agenticon.NewRepository(db)
	for i := 0; i < 99; i++ {
		if err := r.Put(ctx, agenticon.Image{ID: fmt.Sprintf("%032x", i), TenantID: 73, UserID: "synthetic-avatar-user", PNG: []byte("synthetic")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Get(ctx, 74, fmt.Sprintf("%032x", 0)); !errors.Is(err, agenticon.ErrNotFound) {
		t.Fatal("cross-tenant lookup", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 99; i < 109; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results <- r.Put(ctx, agenticon.Image{ID: fmt.Sprintf("%032x", index), TenantID: 73, UserID: "synthetic-avatar-user", PNG: []byte("synthetic")})
		}(i)
	}
	wg.Wait()
	close(results)
	accepted, rejected := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, agenticon.ErrQuota) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 9 {
		t.Fatal("concurrent quota overflow", accepted, rejected)
	}
	if err := runner.Down(ctx, 1); err == nil {
		t.Fatal("rollback erased referenced image storage")
	}
	if _, err := r.Get(ctx, 73, fmt.Sprintf("%032x", 0)); err != nil {
		t.Fatal("blocked rollback changed rows", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM mindcreek.agent_icons WHERE tenant_id=73 AND user_id='synthetic-avatar-user'`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(ctx, 1); err != nil {
		t.Fatal("cleared rollback", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatal("final forward upgrade", err)
	}
}
