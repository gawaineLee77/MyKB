package kbacl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func generation(ctx context.Context, q queryer, tenant uint64, user string) (Generation, error) {
	var g Generation
	err := q.QueryRowContext(ctx, `SELECT COALESCE((SELECT generation FROM mindcreek.kb_acl_generations WHERE tenant_id=$1 AND user_id=$2),0),COALESCE((SELECT generation FROM mindcreek.kb_acl_generations WHERE tenant_id=0 AND user_id=$2),0)`, tenant, user).Scan(&g.Member, &g.Identity)
	return g, err
}
func (r *Repository) Generation(ctx context.Context, tenant uint64, user string) (Generation, error) {
	return generation(ctx, r.db, tenant, user)
}
func readPolicy(ctx context.Context, q queryer, tenant uint64, kb string) (Policy, error) {
	p := Policy{TenantID: tenant, KBID: kb, Members: []Member{}}
	err := q.QueryRowContext(ctx, `SELECT creator_user_id,creator_revoked,creator_member_generation,creator_identity_generation,visibility,revision,deleted FROM mindcreek.kb_policies WHERE tenant_id=$1 AND kb_id=$2`, tenant, kb).Scan(&p.CreatorUserID, &p.CreatorRevoked, &p.CreatorGeneration.Member, &p.CreatorGeneration.Identity, &p.Visibility, &p.Revision, &p.Deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	rows, err := q.QueryContext(ctx, `SELECT user_id,visible,operations,member_generation,identity_generation FROM mindcreek.kb_member_permissions WHERE tenant_id=$1 AND kb_id=$2 ORDER BY user_id`, tenant, kb)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Member
		var ops []byte
		if err := rows.Scan(&m.UserID, &m.Visible, &ops, &m.Generation.Member, &m.Generation.Identity); err != nil {
			return p, err
		}
		if err := json.Unmarshal(ops, &m.Operations); err != nil {
			return p, err
		}
		p.Members = append(p.Members, m)
	}
	return p, rows.Err()
}
func (r *Repository) Policy(ctx context.Context, tenant uint64, kb string) (Policy, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback()
	p, err := readPolicy(ctx, tx, tenant, kb)
	if err != nil {
		return p, err
	}
	return p, tx.Commit()
}
func (r *Repository) Ensure(ctx context.Context, p Policy) (Policy, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO mindcreek.kb_policies(tenant_id,kb_id,creator_user_id,creator_revoked,creator_member_generation,creator_identity_generation,visibility) VALUES($1,$2,$3,$4,$5,$6,'restricted') ON CONFLICT(tenant_id,kb_id) DO NOTHING`, p.TenantID, p.KBID, p.CreatorUserID, p.CreatorRevoked, p.CreatorGeneration.Member, p.CreatorGeneration.Identity)
	if err != nil {
		return Policy{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Policy{}, err
	}
	stored, err := readPolicy(ctx, tx, p.TenantID, p.KBID)
	if err != nil {
		return Policy{}, err
	}
	if count > 0 {
		after, _ := json.Marshal(stored)
		err = insertEvent(ctx, tx, Event{TenantID: p.TenantID, KBID: p.KBID, ActorUserID: auditActor(ctx), Action: "policy.initialize", Revision: stored.Revision, Before: json.RawMessage(`{}`), After: after, Outcome: "success", RequestID: diagnostics.RequestID(ctx)})
		if err != nil {
			return Policy{}, err
		}
	}
	return stored, tx.Commit()
}
func insertEvent(ctx context.Context, tx *sql.Tx, e Event) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO mindcreek.kb_acl_events(tenant_id,kb_id,actor_user_id,action,target,revision,before_state,after_state,outcome,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, e.TenantID, e.KBID, e.ActorUserID, e.Action, e.Target, e.Revision, string(e.Before), string(e.After), e.Outcome, e.RequestID)
	return err
}

// RecordEvent records a guarded content operation without changing its ACL.
// Callers supply only identifiers, operation names and permission metadata.
func (r *Repository) RecordEvent(ctx context.Context, e Event) error {
	if len(e.Before) == 0 {
		e.Before = json.RawMessage(`{}`)
	}
	if len(e.After) == 0 {
		e.After = json.RawMessage(`{}`)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertEvent(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) Save(ctx context.Context, p Policy, expected uint64, e Event) (Policy, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE mindcreek.kb_policies SET visibility=$3,creator_revoked=$4,creator_member_generation=$5,creator_identity_generation=$6,deleted=$7,revision=revision+1,updated_at=now() WHERE tenant_id=$1 AND kb_id=$2 AND revision=$8 AND deleted=false`, p.TenantID, p.KBID, p.Visibility, p.CreatorRevoked, p.CreatorGeneration.Member, p.CreatorGeneration.Identity, p.Deleted, expected)
	if err != nil {
		return Policy{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Policy{}, err
	}
	if count != 1 {
		return Policy{}, ErrConflict
	}
	p.Revision = expected + 1
	if _, err = tx.ExecContext(ctx, `DELETE FROM mindcreek.kb_member_permissions WHERE tenant_id=$1 AND kb_id=$2`, p.TenantID, p.KBID); err != nil {
		return Policy{}, err
	}
	for _, m := range p.Members {
		ops, err := json.Marshal(m.Operations)
		if err != nil {
			return Policy{}, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO mindcreek.kb_member_permissions(tenant_id,kb_id,user_id,visible,operations,member_generation,identity_generation,granted_by,revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.TenantID, p.KBID, m.UserID, m.Visible, string(ops), m.Generation.Member, m.Generation.Identity, e.ActorUserID, p.Revision)
		if err != nil {
			return Policy{}, err
		}
	}
	e.Revision = p.Revision
	e.After, _ = json.Marshal(p)
	if err = insertEvent(ctx, tx, e); err != nil {
		return Policy{}, err
	}
	return p, tx.Commit()
}
func (r *Repository) Events(ctx context.Context, tenant uint64, kb string) ([]Event, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,actor_user_id,action,target,revision,before_state,after_state,outcome,request_id,created_at FROM mindcreek.kb_acl_events WHERE tenant_id=$1 AND kb_id=$2 ORDER BY id DESC LIMIT 100`, tenant, kb)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Event{}
	for rows.Next() {
		e := Event{TenantID: tenant, KBID: kb}
		if err := rows.Scan(&e.ID, &e.ActorUserID, &e.Action, &e.Target, &e.Revision, &e.Before, &e.After, &e.Outcome, &e.RequestID, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (r *Repository) RevokeMember(ctx context.Context, tenant uint64, user string, remove bool) error {
	return r.revoke(ctx, tenant, user, remove, false)
}
func (r *Repository) RevokeIdentity(ctx context.Context, user string) error {
	return r.revoke(ctx, 0, user, true, true)
}
func (r *Repository) revoke(ctx context.Context, tenant uint64, user string, remove, identity bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if remove {
		if _, err = tx.ExecContext(ctx, `INSERT INTO mindcreek.kb_acl_generations(tenant_id,user_id,generation) VALUES($1,$2,1) ON CONFLICT(tenant_id,user_id) DO UPDATE SET generation=kb_acl_generations.generation+1`, tenant, user); err != nil {
			return err
		}
	}
	action := "member.downgrade"
	if remove {
		action = "member.remove"
	}
	if identity {
		action = "identity.revoke"
	}
	// Every affected policy revision advances in the same transaction as its
	// grants and audit, so a stale policy editor cannot restore removed rights.
	rows, err := tx.QueryContext(ctx, `SELECT tenant_id,kb_id FROM mindcreek.kb_policies p WHERE ($1::bigint=0 OR tenant_id=$1) AND deleted=false AND (creator_user_id=$2 OR EXISTS(SELECT 1 FROM mindcreek.kb_member_permissions m WHERE m.tenant_id=p.tenant_id AND m.kb_id=p.kb_id AND m.user_id=$2)) ORDER BY tenant_id,kb_id FOR UPDATE`, tenant, user)
	if err != nil {
		return err
	}
	var affected []Policy
	for rows.Next() {
		var p Policy
		if err = rows.Scan(&p.TenantID, &p.KBID); err != nil {
			rows.Close()
			return err
		}
		affected = append(affected, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i, p := range affected {
		affected[i], err = readPolicy(ctx, tx, p.TenantID, p.KBID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE mindcreek.kb_policies SET creator_revoked=CASE WHEN $3 AND creator_user_id=$4 THEN true ELSE creator_revoked END,revision=revision+1,updated_at=now() WHERE tenant_id=$1 AND kb_id=$2`, p.TenantID, p.KBID, remove, user); err != nil {
			return err
		}
	}
	if remove {
		_, err = tx.ExecContext(ctx, `DELETE FROM mindcreek.kb_member_permissions WHERE ($1::bigint=0 OR tenant_id=$1) AND user_id=$2`, tenant, user)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE mindcreek.kb_member_permissions SET operations='[]'::jsonb WHERE tenant_id=$1 AND user_id=$2`, tenant, user)
	}
	if err != nil {
		return err
	}
	for _, before := range affected {
		after, readErr := readPolicy(ctx, tx, before.TenantID, before.KBID)
		if readErr != nil {
			return readErr
		}
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(after)
		e := Event{TenantID: before.TenantID, KBID: before.KBID, ActorUserID: auditActor(ctx), Action: action, Target: user, Revision: after.Revision, Before: beforeJSON, After: afterJSON, Outcome: "success", RequestID: diagnostics.RequestID(ctx)}
		if err = insertEvent(ctx, tx, e); err != nil {
			return err
		}
	}
	return tx.Commit()
}
