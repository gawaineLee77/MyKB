package revision

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestIncrementAdvancesRevisionAndRecordsSanitizedActivity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO mindcreek.kb_content_revisions").
		WithArgs("kb-1", at).
		WillReturnRows(sqlmock.NewRows([]string{"content_revision"}).AddRow(int64(2)))
	mock.ExpectExec("INSERT INTO mindcreek.kb_activity_events").
		WithArgs(sqlmock.AnyArg(), "kb-1", "alice", "content.uploaded", int64(2), "document accepted", "request-1", at).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	revision, err := repository.Increment(context.Background(), "kb-1", "alice", "content.uploaded", "document accepted", "request-1", at)
	if err != nil {
		t.Fatal(err)
	}
	if revision != 2 {
		t.Fatalf("revision = %d, want 2", revision)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentMapsMissingAndRejectsInvalidInput(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := NewRepository(db)
	if _, err := repository.Current(context.Background(), " "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank KB error = %v, want ErrInvalid", err)
	}
	mock.ExpectQuery("SELECT content_revision").WithArgs("missing").
		WillReturnRows(sqlmock.NewRows([]string{"content_revision"}))
	if _, err := repository.Current(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing revision error = %v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
