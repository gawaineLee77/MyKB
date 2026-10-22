package agenticon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, tenant uint64, id string) (Image, error) {
	i := Image{ID: id, TenantID: tenant}
	err := r.db.QueryRowContext(ctx, `SELECT user_id,png FROM mindcreek.agent_icons WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&i.UserID, &i.PNG)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}
func (r *Repository) Put(ctx context.Context, i Image) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize quota checks and writes on one transaction/connection.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("agent-icon:%d:%s", i.TenantID, i.UserID)); err != nil {
		return err
	}
	var count, size int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(octet_length(png)),0) FROM mindcreek.agent_icons WHERE tenant_id=$1 AND user_id=$2`, i.TenantID, i.UserID).Scan(&count, &size); err != nil {
		return err
	}
	if count >= MaxImages || size+int64(len(i.PNG)) > MaxStoredBytes {
		return ErrQuota
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO mindcreek.agent_icons(id,tenant_id,user_id,png) VALUES ($1,$2,$3,$4)`, i.ID, i.TenantID, i.UserID, i.PNG); err != nil {
		return err
	}
	return tx.Commit()
}
