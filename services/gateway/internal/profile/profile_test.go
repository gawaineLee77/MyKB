package profile

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRepositoryCreateAndLookup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := NewRepository(db)
	candidate := Profile{
		UpstreamKBID: "kb-note-1", TenantID: 42, OwnerUserID: "alice",
		ProductMode: ModePersonalNotes, AccessPolicy: PolicyOwnerOnly,
		IndexProfile: "notes_plain", IndexProfileVersion: 1, EffectiveConfig: []byte(`{"profile_id":"notes_plain"}`),
	}
	mock.ExpectExec("INSERT INTO mindcreek\\.kb_profiles").
		WithArgs(candidate.UpstreamKBID, candidate.TenantID, candidate.OwnerUserID, candidate.ProductMode, 1, candidate.AccessPolicy,
			candidate.IndexProfile, candidate.IndexProfileVersion, []byte(candidate.EffectiveConfig), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	created, err := repository.Create(context.Background(), candidate)
	if err != nil || created.SchemaVersion != 1 || created.CreatedAt.IsZero() {
		t.Fatalf("Create() = %+v, %v", created, err)
	}

	now := time.Now().UTC()
	columns := []string{"upstream_kb_id", "tenant_id", "owner_user_id", "product_mode", "schema_version", "access_policy", "index_profile", "index_profile_version", "effective_config", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM mindcreek.kb_profiles")).WithArgs("kb-note-1").
		WillReturnRows(sqlmock.NewRows(columns).AddRow("kb-note-1", 42, "alice", "personal_notes", 1, "owner_only", "notes_plain", 1, []byte(`{"profile_id":"notes_plain"}`), now, now))
	loaded, err := repository.Get(context.Background(), "kb-note-1")
	if err != nil || loaded.OwnerUserID != "alice" || loaded.TenantID != 42 {
		t.Fatalf("Get() = %+v, %v", loaded, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryUniquenessAndMissing(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repository, _ := NewRepository(db)
	candidate := Profile{UpstreamKBID: "duplicate", TenantID: 42, OwnerUserID: "alice", ProductMode: ModeRAG, AccessPolicy: PolicyUpstream,
		IndexProfile: "plain", IndexProfileVersion: 1, EffectiveConfig: []byte(`{"profile_id":"plain"}`)}
	mock.ExpectExec("INSERT INTO mindcreek\\.kb_profiles").WillReturnError(&pgconn.PgError{Code: "23505"})
	if _, err := repository.Create(context.Background(), candidate); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create() error = %v, want ErrConflict", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("FROM mindcreek.kb_profiles")).WithArgs("missing").WillReturnError(sql.ErrNoRows)
	if _, err := repository.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestForbiddenPersonalNoteIDs(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repository, _ := NewRepository(db)
	mock.ExpectQuery("SELECT upstream_kb_id").WithArgs("bob").
		WillReturnRows(sqlmock.NewRows([]string{"upstream_kb_id"}).AddRow("alice-note").AddRow("carol-note"))
	ids, err := repository.ForbiddenPersonalNoteIDs(context.Background(), "bob")
	if err != nil || len(ids) != 2 {
		t.Fatalf("ForbiddenPersonalNoteIDs() = %v, %v", ids, err)
	}
}

func TestPersonalNotesCannotUseUpstreamPolicy(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	repository, _ := NewRepository(db)
	_, err := repository.Create(context.Background(), Profile{
		UpstreamKBID: "kb", TenantID: 1, OwnerUserID: "alice",
		ProductMode: ModePersonalNotes, AccessPolicy: PolicyUpstream,
	})
	if err == nil {
		t.Fatal("Create() accepted a shareable Personal Notes profile")
	}
}
