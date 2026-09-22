// Package assistant adapts native channels to real employee sessions.
package assistant

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
)

type Binding struct {
	SessionID  string `json:"session_id"`
	ChannelID  string `json:"channel_id"`
	AgentID    string `json:"agent_id"`
	HostOrigin string `json:"host_origin"`
}
type Store interface {
	Create(context.Context, Binding, nativeaccess.Actor) error
	Get(context.Context, string) (Binding, bool, error)
	List(context.Context, string, nativeaccess.Actor) ([]Binding, error)
	Take(context.Context, string, string, int, int) error
}
type Repository struct{ DB *sql.DB }

func (s *Repository) Create(ctx context.Context, b Binding, actor nativeaccess.Actor) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO mindcreek.native_session_bindings(session_id,principal_kind,principal_id,tenant_id) VALUES($1,$2,$3,$4)`, b.SessionID, actor.Kind, actor.ID, actor.TenantID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mindcreek.assistant_sessions(session_id,channel_id,agent_id,host_origin) VALUES($1,$2,$3,$4)`, b.SessionID, b.ChannelID, b.AgentID, b.HostOrigin)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Repository) Get(ctx context.Context, id string) (Binding, bool, error) {
	var b Binding
	err := s.DB.QueryRowContext(ctx, `SELECT session_id,channel_id,agent_id,host_origin FROM mindcreek.assistant_sessions WHERE session_id=$1`, id).Scan(&b.SessionID, &b.ChannelID, &b.AgentID, &b.HostOrigin)
	if errors.Is(err, sql.ErrNoRows) {
		return b, false, nil
	}
	return b, err == nil, err
}
func (s *Repository) List(ctx context.Context, channel string, actor nativeaccess.Actor) ([]Binding, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.session_id,a.channel_id,a.agent_id,a.host_origin FROM mindcreek.assistant_sessions a JOIN mindcreek.native_session_bindings b USING(session_id) WHERE a.channel_id=$1 AND b.tenant_id=$2 AND b.principal_kind=$3 AND b.principal_id=$4 ORDER BY a.created_at DESC LIMIT 100`, channel, actor.TenantID, actor.Kind, actor.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Binding{}
	for rows.Next() {
		var b Binding
		if err := rows.Scan(&b.SessionID, &b.ChannelID, &b.AgentID, &b.HostOrigin); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// A transaction and channel lock enforce both budgets across gateway replicas.
func (s *Repository) Take(ctx context.Context, channel, user string, minute, day int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "mindcreek:assistant:"+channel); err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT now()`).Scan(&now); err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(user))
	user = hex.EncodeToString(hash[:])
	for _, w := range []struct {
		key      string
		duration time.Duration
		limit    int
	}{{channel + ":" + user, time.Minute, minute}, {channel, 24 * time.Hour, day}} {
		start := now.UTC().Truncate(w.duration)
		bucket := fmt.Sprintf("%s:%d:%d", w.key, int64(w.duration/time.Second), start.Unix())
		var used int
		err = tx.QueryRowContext(ctx, `INSERT INTO mindcreek.assistant_rate_windows(bucket,used,expires_at) VALUES($1,1,$2) ON CONFLICT(bucket) DO UPDATE SET used=mindcreek.assistant_rate_windows.used+1 RETURNING used`, bucket, start.Add(w.duration)).Scan(&used)
		if err != nil {
			return err
		}
		if used > w.limit {
			return &nativeaccess.Error{Status: 429, Code: "assistant.rate_limited"}
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM mindcreek.assistant_rate_windows WHERE expires_at < now() - interval '1 day'`); err != nil {
		return err
	}
	return tx.Commit()
}
