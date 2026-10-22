package kbacl

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStalePolicySaveNeverDeletesCurrentGrants(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mindcreek.kb_policies SET.*WHERE tenant_id=\$1 AND kb_id=\$2 AND revision=\$8 AND deleted=false`).WithArgs(uint64(7), "kb", "restricted", false, uint64(0), uint64(0), false, uint64(2)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	_, err = NewRepository(db).Save(context.Background(), Policy{TenantID: 7, KBID: "kb", Visibility: "restricted"}, 2, Event{})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPolicyAndMemberChangesRollbackWhenAuditCannotPersist(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mindcreek.kb_policies SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM mindcreek.kb_member_permissions`).WithArgs(uint64(7), "kb").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO mindcreek.kb_member_permissions`).WithArgs(uint64(7), "kb", "collaborator", true, `["chunk.update"]`, uint64(2), uint64(4), "creator", uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO mindcreek.kb_acl_events`).WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()
	p := Policy{TenantID: 7, KBID: "kb", Visibility: "restricted", Members: []Member{{UserID: "collaborator", Visible: true, Operations: []string{"chunk.update"}, Generation: Generation{Member: 2, Identity: 4}}}}
	_, err = NewRepository(db).Save(context.Background(), p, 2, Event{ActorUserID: "creator"})
	if err == nil {
		t.Fatal("unaudited mutation committed")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestIdentityRevocationPersistsGenerationWithoutAnyExistingPolicy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO mindcreek.kb_acl_generations.*generation=kb_acl_generations.generation\+1`).WithArgs(uint64(0), "employee").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT tenant_id,kb_id FROM mindcreek.kb_policies p .*ORDER BY tenant_id,kb_id FOR UPDATE`).WithArgs(uint64(0), "employee").WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "kb_id"}))
	mock.ExpectExec(`DELETE FROM mindcreek.kb_member_permissions WHERE`).WithArgs(uint64(0), "employee").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err = NewRepository(db).RevokeIdentity(context.Background(), "employee"); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCreationCompletionRollsBackIfPolicyBindingMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mindcreek.kb_creation_operations SET`).WithArgs(uint64(7), "operation", "kb").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT revision FROM mindcreek.kb_policies.*deleted=false`).WithArgs(uint64(7), "kb").WillReturnError(errors.New("binding missing"))
	mock.ExpectRollback()
	if err = NewRepository(db).CompleteCreation(context.Background(), 7, "operation", "kb"); err == nil {
		t.Fatal("creation became usable without policy")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
