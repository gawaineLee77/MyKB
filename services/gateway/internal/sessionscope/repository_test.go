package sessionscope

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRepositoryRecordsUnionAndListsScopes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustRepository(t, db)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO mindcreek.session_kb_scopes")).
		WithArgs("session-1", "kb-1", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.RecordKnowledgeBases(context.Background(), "session-1", []string{"kb-1", "kb-1"}, now); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT knowledge_base_id").WithArgs("session-1").
		WillReturnRows(sqlmock.NewRows([]string{"knowledge_base_id"}).AddRow("kb-1"))
	items, err := repository.ListKnowledgeBases(context.Background(), "session-1")
	if err != nil || len(items) != 1 || items[0] != "kb-1" {
		t.Fatalf("ListKnowledgeBases() = %+v, %v", items, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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
