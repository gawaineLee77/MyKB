package audit

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRepositoryRecordsRedactedEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustAuditRepository(t, db)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO mindcreek.kb_access_audit_events")).
		WithArgs(sqlmock.AnyArg(), uint64(42), "kb-1", "alice", "grant.create", "kb_grant", "grant-1",
			OutcomeSuccess, "", "request-1", nil, []byte(`{"permission":"viewer"}`), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	err = repository.Record(context.Background(), Event{
		TenantID: 42, KnowledgeBaseID: "kb-1", ActorUserID: "alice", Action: "grant.create",
		TargetType: "kb_grant", TargetID: "grant-1", Outcome: OutcomeSuccess,
		CorrelationID: "request-1", NewValue: []byte(`{"permission":"viewer"}`), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEventRejectsSensitiveOrInvalidPayloadShape(t *testing.T) {
	event := Event{
		ID: "event-1", TenantID: 42, KnowledgeBaseID: "kb-1", ActorUserID: "alice",
		Action: "grant.create", TargetType: "kb_grant", TargetID: "grant-1",
		Outcome: OutcomeSuccess, CorrelationID: "request-1", CreatedAt: time.Now(),
		NewValue: []byte(`"raw prompt content"`),
	}
	if err := event.validate(); err == nil {
		t.Fatal("audit event accepted a non-object payload")
	}
}

func mustAuditRepository(t *testing.T, db *sql.DB) *Repository {
	t.Helper()
	repository, err := NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
