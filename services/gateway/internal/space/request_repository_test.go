package space

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRequestRepositoryClaimIsStableAndIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository, _ := NewRequestRepository(db)
	now := time.Now().UTC()
	candidate := CreationRequest{
		TenantID: 42, OwnerUserID: "alice", IdempotencyKey: "create-note-1",
		RequestHash: stringOf('a', 64), UpstreamKBID: "kb-deterministic",
		ProductMode: "personal_notes", IndexProfile: "notes_plain",
	}
	columns := []string{
		"tenant_id", "owner_user_id", "idempotency_key", "request_hash", "upstream_kb_id",
		"product_mode", "index_profile", "status", "last_error", "created_at", "updated_at",
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO mindcreek.knowledge_space_requests").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM mindcreek.knowledge_space_requests").
		WithArgs(candidate.TenantID, candidate.OwnerUserID, candidate.IdempotencyKey).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			42, "alice", "create-note-1", candidate.RequestHash, "kb-deterministic",
			"personal_notes", "notes_plain", "pending", "", now, now,
		))
	mock.ExpectCommit()
	stored, inserted, err := repository.Claim(context.Background(), candidate)
	if err != nil || !inserted || stored.UpstreamKBID != candidate.UpstreamKBID {
		t.Fatalf("Claim() = %+v, %t, %v", stored, inserted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestRepositoryRejectsKeyReuseWithDifferentHash(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repository, _ := NewRequestRepository(db)
	candidate := CreationRequest{
		TenantID: 42, OwnerUserID: "alice", IdempotencyKey: "same-key",
		RequestHash: stringOf('a', 64), UpstreamKBID: "new-kb",
		ProductMode: "personal_notes", IndexProfile: "notes_plain",
	}
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO mindcreek.knowledge_space_requests").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM mindcreek.knowledge_space_requests").WillReturnRows(sqlmock.NewRows([]string{
		"tenant_id", "owner_user_id", "idempotency_key", "request_hash", "upstream_kb_id",
		"product_mode", "index_profile", "status", "last_error", "created_at", "updated_at",
	}).AddRow(42, "alice", "same-key", stringOf('b', 64), "existing-kb", "personal_notes", "notes_plain", "ready", "", now, now))
	mock.ExpectRollback()
	_, _, err := repository.Claim(context.Background(), candidate)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Claim() error = %v", err)
	}
}

func TestRequestRepositoryCompletesAndRecordsFailure(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repository, _ := NewRequestRepository(db)
	request := CreationRequest{
		TenantID: 42, OwnerUserID: "alice", IdempotencyKey: "key",
		RequestHash: stringOf('a', 64), UpstreamKBID: "kb",
	}
	mock.ExpectExec("UPDATE mindcreek.knowledge_space_requests").
		WithArgs(uint64(42), "alice", "key", request.RequestHash, "kb").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE mindcreek.knowledge_space_requests").
		WithArgs(uint64(42), "alice", "key", request.RequestHash, "kb", "upstream failed").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.Fail(context.Background(), request, "upstream failed"); err != nil {
		t.Fatal(err)
	}
}

func stringOf(value rune, count int) string {
	result := make([]rune, count)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
