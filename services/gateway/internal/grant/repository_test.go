package grant

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

var grantColumns = []string{
	"id", "knowledge_base_id", "subject_type", "subject_id", "permission", "granted_by",
	"created_at", "updated_at", "expires_at", "revoked_at", "revision", "last_audit_correlation_id",
}

func TestRepositoryCreateAndEffectiveUserGrant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustRepository(t, db)
	now := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	candidate := Grant{
		ID: "grant-1", KnowledgeBaseID: "kb-1", SubjectType: SubjectUser, SubjectID: "bob",
		Permission: PermissionViewer, GrantedBy: "alice", CreatedAt: now, UpdatedAt: now,
		Revision: 1, LastAuditCorrelationID: "request-1",
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO mindcreek.kb_access_grants")).
		WithArgs(
			candidate.ID, candidate.KnowledgeBaseID, candidate.SubjectType, candidate.SubjectID,
			candidate.Permission, candidate.GrantedBy, candidate.CreatedAt, candidate.UpdatedAt,
			candidate.ExpiresAt, candidate.RevokedAt, candidate.Revision, candidate.LastAuditCorrelationID,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	created, err := repository.Create(context.Background(), candidate)
	if err != nil || created.ID != "grant-1" {
		t.Fatalf("Create() = %+v, %v", created, err)
	}

	mock.ExpectQuery("SELECT id, knowledge_base_id, subject_type").
		WithArgs("kb-1", "bob", now).
		WillReturnRows(sqlmock.NewRows(grantColumns).AddRow(
			"grant-1", "kb-1", "user", "bob", "viewer", "alice",
			now, now, nil, nil, int64(1), "request-1",
		))
	effective, err := repository.EffectiveUserGrant(context.Background(), "kb-1", "bob", now)
	if err != nil || effective.Permission != PermissionViewer || !effective.Active(now) {
		t.Fatalf("EffectiveUserGrant() = %+v, %v", effective, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryTranslatesCreateConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustRepository(t, db)
	now := time.Now().UTC()
	candidate := Grant{
		ID: "grant-1", KnowledgeBaseID: "kb-1", SubjectType: SubjectUser, SubjectID: "bob",
		Permission: PermissionViewer, GrantedBy: "alice", CreatedAt: now, UpdatedAt: now,
		Revision: 1, LastAuditCorrelationID: "request-1",
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO mindcreek.kb_access_grants")).
		WithArgs(
			candidate.ID, candidate.KnowledgeBaseID, candidate.SubjectType, candidate.SubjectID,
			candidate.Permission, candidate.GrantedBy, candidate.CreatedAt, candidate.UpdatedAt,
			candidate.ExpiresAt, candidate.RevokedAt, candidate.Revision, candidate.LastAuditCorrelationID,
		).
		WillReturnError(&pgconn.PgError{Code: "23505"})
	if _, err := repository.Create(context.Background(), candidate); !errors.Is(err, ErrConflict) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestRepositoryUpdateRequiresMatchingRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustRepository(t, db)
	now := time.Now().UTC()
	mock.ExpectQuery("UPDATE mindcreek.kb_access_grants").
		WithArgs("grant-1", int64(3), PermissionEditor, nil, now, "request-2").
		WillReturnError(sql.ErrNoRows)
	_, err = repository.Update(context.Background(), "grant-1", 3, PermissionEditor, nil, "request-2", now)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Update() error = %v", err)
	}
}

func mustRepository(t *testing.T, db *sql.DB) *Repository {
	t.Helper()
	repository, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
