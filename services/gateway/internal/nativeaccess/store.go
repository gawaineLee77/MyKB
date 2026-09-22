package nativeaccess

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type Binding struct {
	SessionID string
	Actor     Actor
	KBIDs     []string
}
type SessionStore interface {
	Bind(context.Context, string, Actor, []string) error
	Binding(context.Context, string) (Binding, error)
	Bindings(context.Context, Actor) ([]Binding, error)
	Record(context.Context, Actor, string, string, string, []string, string) error
}

type Repository struct{ DB *sql.DB }

func (r *Repository) Bindings(ctx context.Context, actor Actor) ([]Binding, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT session_id,knowledge_base_ids FROM mindcreek.native_session_bindings WHERE tenant_id=$1 AND principal_kind=$2 AND principal_id=$3 ORDER BY created_at DESC LIMIT 10001`, actor.TenantID, actor.Kind, actor.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Binding{}
	for rows.Next() {
		b := Binding{Actor: actor}
		var raw []byte
		if err := rows.Scan(&b.SessionID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &b.KBIDs); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	if len(result) > 10000 {
		return nil, &Error{422, "session.history_limit"}
	}
	return result, rows.Err()
}

func (r *Repository) Binding(ctx context.Context, id string) (Binding, error) {
	var result Binding
	var raw []byte
	err := r.DB.QueryRowContext(ctx, `SELECT session_id, principal_kind, principal_id, tenant_id, knowledge_base_ids FROM mindcreek.native_session_bindings WHERE session_id=$1`, id).Scan(&result.SessionID, &result.Actor.Kind, &result.Actor.ID, &result.Actor.TenantID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Binding{}, &Error{403, "session.unbound"}
	}
	if err != nil {
		return Binding{}, err
	}
	if err := json.Unmarshal(raw, &result.KBIDs); err != nil {
		return Binding{}, err
	}
	return result, nil
}

// Bind atomically creates a new binding or extends the same principal's union.
// It can never adopt a session bound to another human, key, or workspace.
func (r *Repository) Bind(ctx context.Context, id string, actor Actor, ids []string) error {
	if id == "" || len(id) > 128 || actor.ID == "" || actor.TenantID == 0 || (actor.Kind != "human" && actor.Kind != "api_key") {
		return fmt.Errorf("invalid session binding")
	}
	if ids == nil {
		ids = []string{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	result, err := r.DB.ExecContext(ctx, `INSERT INTO mindcreek.native_session_bindings (session_id,principal_kind,principal_id,tenant_id,knowledge_base_ids)
VALUES ($1,$2,$3,$4,$5) ON CONFLICT (session_id) DO UPDATE SET
knowledge_base_ids=(SELECT COALESCE(jsonb_agg(DISTINCT value),'[]'::jsonb) FROM jsonb_array_elements(mindcreek.native_session_bindings.knowledge_base_ids || EXCLUDED.knowledge_base_ids)), updated_at=now()
WHERE mindcreek.native_session_bindings.principal_kind=EXCLUDED.principal_kind AND mindcreek.native_session_bindings.principal_id=EXCLUDED.principal_id AND mindcreek.native_session_bindings.tenant_id=EXCLUDED.tenant_id`, id, actor.Kind, actor.ID, actor.TenantID, raw)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return &Error{403, "session.principal_mismatch"}
	}
	return nil
}

func (r *Repository) Record(ctx context.Context, actor Actor, operation, outcome, code string, ids []string, correlation string) error {
	if ids == nil {
		ids = []string{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	if len(correlation) > 128 {
		correlation = ""
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO mindcreek.native_access_events (principal_kind,principal_id,tenant_id,operation,outcome,error_code,knowledge_base_ids,correlation_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, actor.Kind, actor.ID, actor.TenantID, operation, outcome, code, raw, correlation)
	return err
}
