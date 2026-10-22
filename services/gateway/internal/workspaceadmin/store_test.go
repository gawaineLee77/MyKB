package workspaceadmin

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRepositoryCreatorDisableBumpsGenerationAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db, db)
	// One statement advances the tombstone even when no key exists yet. It
	// also disables already-issued keys, closing a concurrent create race.
	mock.ExpectExec(`WITH bumped AS \(INSERT INTO mindcreek.workspace_api_key_creator_generations.*ON CONFLICT\(user_id\).*generation\+1.*UPDATE mindcreek.workspace_api_key_owners SET disabled=true WHERE creator_user_id=\$1`).WithArgs("employee").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := repo.DisableUserKeys(context.Background(), 0, "employee"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE((SELECT generation FROM mindcreek.workspace_api_key_creator_generations WHERE user_id=$1),0)`)).WithArgs("employee").WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(3))
	generation, err := repo.CreatorGeneration(context.Background(), "employee")
	if err != nil || generation != 3 {
		t.Fatalf("generation %d, %v", generation, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestRepositoryKeyLookupUsesTenantAndPreservesFingerprintTombstone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db, db)
	fp := strings.Repeat("a", 64)
	mock.ExpectQuery(regexp.QuoteMeta(keySelect+` WHERE tenant_id=$1 AND key_id=$2`)).WithArgs(uint64(2), uint64(4)).WillReturnError(sql.ErrNoRows)
	if _, err := repo.Key(context.Background(), 2, 4); err != ErrNotFound {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(keySelect + ` WHERE fingerprint=$1`)).WithArgs(fp).WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "key_id", "creator_user_id", "required_role", "fingerprint", "disabled", "creator_generation"}).AddRow(1, 4, "employee", "admin", fp, true, 2))
	key, err := repo.KeyByFingerprint(context.Background(), fp)
	if err != nil || !key.Disabled || key.TenantID != 1 || key.CreatorGeneration != 2 {
		t.Fatalf("key %#v %v", key, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestRepositoryPendingCreateStoresRecoveryIDBeforeRename(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db, db)
	mock.ExpectExec(`INSERT INTO mindcreek.workspace_api_key_pending.*ON CONFLICT\(tenant_id,marker\) DO UPDATE SET key_id=EXCLUDED.key_id`).WithArgs(uint64(1), "marker", uint64(88)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.SavePending(context.Background(), 1, "marker", 88); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT marker,key_id FROM mindcreek.workspace_api_key_pending WHERE tenant_id=\$1`).WithArgs(uint64(1)).WillReturnRows(sqlmock.NewRows([]string{"marker", "key_id"}).AddRow("marker", 88))
	pending, err := repo.PendingCreates(context.Background(), 1)
	if err != nil || len(pending) != 1 || pending[0].KeyID != 88 {
		t.Fatalf("pending %#v %v", pending, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositorySeparatesAdvisoryLocksFromPolicyQueryPool(t *testing.T) {
	db, queries, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	locksDB, locks, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer locksDB.Close()
	db.SetMaxOpenConns(1)
	locksDB.SetMaxOpenConns(1)
	locks.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_lock(hashtextextended($1, 20261016))`)).WithArgs("tenant").WillReturnResult(sqlmock.NewResult(0, 1))
	queries.ExpectQuery(`SELECT COALESCE\(\(SELECT generation`).WithArgs("employee").WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(2))
	locks.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock(hashtextextended($1, 20261016))`)).WithArgs("tenant").WillReturnResult(sqlmock.NewResult(0, 1))
	repo := NewRepository(db, locksDB)
	if err := repo.WithLock(context.Background(), "tenant", func(store Store) error {
		generation, err := store.CreatorGeneration(context.Background(), "employee")
		if generation != 2 {
			t.Errorf("unexpected generation %d", generation)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := locks.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := NewRepository(db, db).WithLock(context.Background(), "tenant", func(Store) error {
		t.Fatal("same pool admitted a potentially deadlocking operation")
		return nil
	}); err == nil {
		t.Fatal("accepted same query and lock pool")
	}
}

func TestRepositoryDiscardsLockSessionAfterAmbiguousAcquisition(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	locksDB, locks, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer locksDB.Close()
	locks.ExpectExec(`SELECT pg_advisory_lock`).WithArgs("tenant").WillReturnError(context.DeadlineExceeded)
	locks.ExpectClose()
	err = NewRepository(db, locksDB).WithLock(context.Background(), "tenant", func(Store) error {
		t.Fatal("operation ran after ambiguous lock acquisition")
		return nil
	})
	if err == nil {
		t.Fatal("ignored acquisition failure")
	}
	if err := locks.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
