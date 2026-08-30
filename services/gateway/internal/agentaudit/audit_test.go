package agentaudit

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRepositoryRecordsOnlyRedactedMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustRepository(t, db)
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO mindcreek.agent_operation_audit_events")).
		WithArgs(sqlmock.AnyArg(), uint64(42), "alice", ClientMCP, "search_knowledge", []byte(`["kb-a","kb-b"]`),
			OutcomeSuccess, "", "request-1", int64(125), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	err = repository.Record(context.Background(), Event{
		TenantID: 42, ActorUserID: "alice", ClientKind: ClientMCP, Operation: "search_knowledge",
		KnowledgeBaseIDs: []string{"kb-b", "kb-a"}, Outcome: OutcomeSuccess, CorrelationID: "request-1",
		Duration: 125 * time.Millisecond, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRejectsDuplicateOrOversizedScope(t *testing.T) {
	event := Event{ID: "event-1", TenantID: 42, ActorUserID: "alice", ClientKind: ClientWeb,
		Operation: "agent.query", KnowledgeBaseIDs: []string{"kb-a", "kb-a"}, Outcome: OutcomeSuccess,
		CorrelationID: "request-1", CreatedAt: time.Now()}
	if err := event.validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want invalid", err)
	}
}

func TestRepositoryRecordsEmptyScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := mustRepository(t, db)
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO mindcreek.agent_operation_audit_events")).
		WithArgs(sqlmock.AnyArg(), uint64(42), "alice", ClientMCP, "list_subscriptions", []byte(`[]`),
			OutcomeSuccess, "", "request-empty", int64(0), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.Record(context.Background(), Event{
		TenantID: 42, ActorUserID: "alice", ClientKind: ClientMCP, Operation: "list_subscriptions",
		KnowledgeBaseIDs: []string{}, Outcome: OutcomeSuccess, CorrelationID: "request-empty", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
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
