package branding

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
)

type Repository struct{ DB *sql.DB }

func (r *Repository) Get(ctx context.Context) (Record, error) {
	var out Record
	err := r.DB.QueryRowContext(ctx, `SELECT name,logo,revision FROM mindcreek.service_branding WHERE id=1`).Scan(&out.Name, &out.Logo, &out.Revision)
	return out, err
}
func (r *Repository) Update(ctx context.Context, c Change, actor, requestID string) (Record, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback()
	var before Record
	if err = tx.QueryRowContext(ctx, `SELECT name,logo,revision FROM mindcreek.service_branding WHERE id=1 FOR UPDATE`).Scan(&before.Name, &before.Logo, &before.Revision); err != nil {
		return Record{}, err
	}
	if before.Revision != c.Revision {
		return Record{}, ErrConflict
	}
	after := Record{Name: c.Name, Logo: before.Logo, Revision: before.Revision + 1}
	if c.ReplaceLogo {
		after.Logo = c.Logo
	}
	if after.Logo == nil {
		after.Logo = []byte{}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE mindcreek.service_branding SET name=$1,logo=$2,revision=$3,updated_by=$4,updated_at=now() WHERE id=1`, after.Name, after.Logo, after.Revision, actor); err != nil {
		return Record{}, err
	}
	metadata := func(value Record) []byte {
		hash := ""
		if len(value.Logo) > 0 {
			sum := sha256.Sum256(value.Logo)
			hash = hex.EncodeToString(sum[:])
		}
		data, _ := json.Marshal(map[string]any{"name": value.Name, "revision": value.Revision, "logo_sha256": hash})
		return data
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO mindcreek.service_branding_events (actor_user_id,request_id,before_state,after_state) VALUES ($1,$2,$3,$4)`, actor, requestID, metadata(before), metadata(after)); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(); err != nil {
		return Record{}, err
	}
	return after, nil
}
