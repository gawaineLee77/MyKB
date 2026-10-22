// Package workspaceadmin adds human workspace administration outside upstream.
package workspaceadmin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("workspace administration record not found")

type Delegation struct {
	TenantID        uint64
	OwnerUserID     string
	SessionCipher   []byte
	MemberKeyID     uint64
	MemberKeyCipher []byte
	UpdatedAt       time.Time
}
type KeyOwner struct {
	CreatorGeneration                        uint64
	TenantID, KeyID                          uint64
	CreatorUserID, RequiredRole, Fingerprint string
	Disabled                                 bool
}
type PendingCreate struct {
	Marker string
	KeyID  uint64
}
type Audit struct {
	CorrelationID                        string
	TenantID                             uint64
	ActorUserID, Action, Target, Outcome string
}
type Store interface {
	WithLock(context.Context, string, func(Store) error) error
	Delegation(context.Context, uint64) (Delegation, error)
	SaveDelegation(context.Context, Delegation) error
	DeleteDelegation(context.Context, uint64) error
	Key(context.Context, uint64, uint64) (KeyOwner, error)
	KeyByFingerprint(context.Context, string) (KeyOwner, error)
	SaveKey(context.Context, KeyOwner) error
	DisableKey(context.Context, uint64, uint64) error
	DisableUserKeys(context.Context, uint64, string) error
	CreatorGeneration(context.Context, string) (uint64, error)
	PendingCreates(context.Context, uint64) ([]PendingCreate, error)
	SavePending(context.Context, uint64, string, uint64) error
	DeletePending(context.Context, uint64, string) error
	RecordAudit(context.Context, Audit) error
}
type queries interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type Repository struct {
	db     *sql.DB
	lockDB *sql.DB
	q      queries
}

func NewRepository(db, lockDB *sql.DB) *Repository { return &Repository{db: db, lockDB: lockDB, q: db} }
func (r *Repository) WithLock(ctx context.Context, key string, fn func(Store) error) error {
	return r.WithLocks(ctx, []string{key}, fn)
}

// WithLocks keeps the ordered lock chain on one connection. Acquiring each
// level from a fresh pool connection can deadlock when concurrent waiters
// exhaust the pool before an existing holder obtains its next lock.
func (r *Repository) WithLocks(ctx context.Context, keys []string, fn func(Store) error) error {
	if r.db == nil || r.lockDB == nil || r.db == r.lockDB {
		return fmt.Errorf("workspace administration requires distinct query and lock pools")
	}
	conn, err := r.lockDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	seen := map[string]bool{}
	held := []string{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 20261016))`, key); err != nil {
			// Cancellation can lose a successful acquisition response. Discard
			// the session, releasing every lock it might have acquired.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return err
		}
		held = append(held, key)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := len(held) - 1; i >= 0; i-- {
			if _, err := conn.ExecContext(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1, 20261016))`, held[i]); err != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				break
			}
		}
	}()
	// Queries and policy callbacks use the ordinary pool; blocked advisory
	// lock waiters can never consume the connections needed by the holder.
	return fn(&Repository{db: r.db, lockDB: r.lockDB, q: r.db})
}
func (r *Repository) Delegation(ctx context.Context, id uint64) (Delegation, error) {
	var d Delegation
	err := r.q.QueryRowContext(ctx, `SELECT tenant_id,owner_user_id,session_cipher,member_key_id,member_key_cipher,updated_at FROM mindcreek.workspace_admin_delegations WHERE tenant_id=$1`, id).Scan(&d.TenantID, &d.OwnerUserID, &d.SessionCipher, &d.MemberKeyID, &d.MemberKeyCipher, &d.UpdatedAt)
	return d, notFound(err)
}
func (r *Repository) SaveDelegation(ctx context.Context, d Delegation) error {
	if d.MemberKeyCipher == nil {
		d.MemberKeyCipher = []byte{}
	}
	_, err := r.q.ExecContext(ctx, `INSERT INTO mindcreek.workspace_admin_delegations(tenant_id,owner_user_id,session_cipher,member_key_id,member_key_cipher) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id) DO UPDATE SET owner_user_id=EXCLUDED.owner_user_id,session_cipher=EXCLUDED.session_cipher,member_key_id=EXCLUDED.member_key_id,member_key_cipher=EXCLUDED.member_key_cipher,updated_at=now()`, d.TenantID, d.OwnerUserID, d.SessionCipher, d.MemberKeyID, d.MemberKeyCipher)
	return err
}
func (r *Repository) DeleteDelegation(ctx context.Context, id uint64) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM mindcreek.workspace_admin_delegations WHERE tenant_id=$1`, id)
	return err
}

const keySelect = `SELECT tenant_id,key_id,creator_user_id,required_role,fingerprint,disabled,creator_generation FROM mindcreek.workspace_api_key_owners`

func scanKey(row *sql.Row) (KeyOwner, error) {
	var k KeyOwner
	err := row.Scan(&k.TenantID, &k.KeyID, &k.CreatorUserID, &k.RequiredRole, &k.Fingerprint, &k.Disabled, &k.CreatorGeneration)
	return k, notFound(err)
}
func (r *Repository) Key(ctx context.Context, tenant, key uint64) (KeyOwner, error) {
	return scanKey(r.q.QueryRowContext(ctx, keySelect+` WHERE tenant_id=$1 AND key_id=$2`, tenant, key))
}
func (r *Repository) KeyByFingerprint(ctx context.Context, f string) (KeyOwner, error) {
	return scanKey(r.q.QueryRowContext(ctx, keySelect+` WHERE fingerprint=$1`, f))
}
func (r *Repository) SaveKey(ctx context.Context, k KeyOwner) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO mindcreek.workspace_api_key_owners(tenant_id,key_id,creator_user_id,required_role,fingerprint,disabled,creator_generation) VALUES($1,$2,$3,$4,$5,$6,$7)`, k.TenantID, k.KeyID, k.CreatorUserID, k.RequiredRole, k.Fingerprint, k.Disabled, k.CreatorGeneration)
	return err
}
func (r *Repository) DisableKey(ctx context.Context, tenant, key uint64) error {
	_, err := r.q.ExecContext(ctx, `UPDATE mindcreek.workspace_api_key_owners SET disabled=true WHERE tenant_id=$1 AND key_id=$2`, tenant, key)
	return err
}
func (r *Repository) DisableUserKeys(ctx context.Context, tenant uint64, user string) error {
	if tenant == 0 {
		_, err := r.q.ExecContext(ctx, `WITH bumped AS (INSERT INTO mindcreek.workspace_api_key_creator_generations(user_id,generation) VALUES($1,1) ON CONFLICT(user_id) DO UPDATE SET generation=workspace_api_key_creator_generations.generation+1 RETURNING generation) UPDATE mindcreek.workspace_api_key_owners SET disabled=true WHERE creator_user_id=$1 AND (SELECT generation FROM bumped)>0`, user)
		return err
	}

	_, err := r.q.ExecContext(ctx, `UPDATE mindcreek.workspace_api_key_owners SET disabled=true WHERE ($1::bigint=0 OR tenant_id=$1) AND creator_user_id=$2`, tenant, user)
	return err
}
func (r *Repository) PendingCreates(ctx context.Context, id uint64) ([]PendingCreate, error) {
	rows, err := r.q.QueryContext(ctx, `SELECT marker,key_id FROM mindcreek.workspace_api_key_pending WHERE tenant_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PendingCreate{}
	for rows.Next() {
		var v PendingCreate
		if err := rows.Scan(&v.Marker, &v.KeyID); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (r *Repository) SavePending(ctx context.Context, id uint64, marker string, keyID uint64) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO mindcreek.workspace_api_key_pending(tenant_id,marker,key_id) VALUES($1,$2,$3) ON CONFLICT(tenant_id,marker) DO UPDATE SET key_id=EXCLUDED.key_id`, id, marker, keyID)
	return err
}
func (r *Repository) DeletePending(ctx context.Context, id uint64, marker string) error {
	_, err := r.q.ExecContext(ctx, `DELETE FROM mindcreek.workspace_api_key_pending WHERE tenant_id=$1 AND marker=$2`, id, marker)
	return err
}
func (r *Repository) RecordAudit(ctx context.Context, a Audit) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO mindcreek.workspace_admin_audit(tenant_id,actor_user_id,action,target,outcome,correlation_id) VALUES($1,$2,$3,$4,$5,$6)`, a.TenantID, a.ActorUserID, a.Action, a.Target, a.Outcome, a.CorrelationID)
	return err
}
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *Repository) CreatorGeneration(ctx context.Context, user string) (uint64, error) {
	var generation uint64
	err := r.q.QueryRowContext(ctx, `SELECT COALESCE((SELECT generation FROM mindcreek.workspace_api_key_creator_generations WHERE user_id=$1),0)`, user).Scan(&generation)
	return generation, err
}
