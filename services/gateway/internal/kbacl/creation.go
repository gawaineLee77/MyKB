package kbacl

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
)

const creationPrefix = "/api/v1/mindcreek/kb-creation-operations"

func hashKey(key string) string { sum := sha256.Sum256([]byte(key)); return hex.EncodeToString(sum[:]) }
func validHash(v string) bool {
	b, err := hex.DecodeString(v)
	return err == nil && len(b) == 32 && v == strings.ToLower(v)
}

// The pinned native endpoint returns the entire current-tenant array with no
// pagination and no creator filter. Reject a changed/paginated contract rather
// than silently treating an incomplete inventory as a creation baseline.
func (s *Service) creationBaseline(ctx context.Context, h http.Header, tenant uint64) ([]string, error) {
	raw, err := s.Upstream.NativeRequest(ctx, http.MethodGet, "/api/v1/knowledge-bases", nil, h, nil)
	if err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	var success bool
	var rows []kbIdentity
	if json.Unmarshal(raw, &envelope) != nil || len(envelope) != 2 || json.Unmarshal(envelope["success"], &success) != nil || !success || json.Unmarshal(envelope["data"], &rows) != nil || rows == nil {
		return nil, fail(502, "creation_inventory_invalid")
	}
	ids := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		if !validID(row.ID) || row.TenantID != tenant || seen[row.ID] {
			return nil, fail(502, "creation_inventory_invalid")
		}
		seen[row.ID] = true
		ids = append(ids, row.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

// BeginCreate must precede native creation and run inside the guard's locks.
// An uncertain outcome is retained indefinitely and never blindly replayed.
func (s *Service) BeginCreate(ctx context.Context, h http.Header, key, requestHash string) (CreateOperation, error) {
	p, err := s.principal(ctx, h, true)
	if err != nil {
		return CreateOperation{}, err
	}
	if !isEditor(nativeRole(p)) {
		return CreateOperation{}, fail(403, "operation_denied")
	}
	if !validID(key) || !validHash(requestHash) {
		return CreateOperation{}, fail(400, "idempotency_required")
	}
	baseline, err := s.creationBaseline(ctx, h, p.Tenant.ID)
	if err != nil {
		return CreateOperation{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return CreateOperation{}, err
	}
	op := CreateOperation{ID: hex.EncodeToString(id[:]), TenantID: p.Tenant.ID, ActorUserID: p.User.ID, KeyHash: hashKey(key), RequestHash: requestHash, State: "pending", RequestID: diagnostics.RequestID(ctx), BaselineIDs: baseline}
	stored, created, err := s.Store.BeginCreation(ctx, op)
	if err != nil {
		return stored, err
	}
	if !created {
		if stored.RequestHash != requestHash {
			return stored, fail(409, "idempotency_conflict")
		}
		if stored.State == "completed" {
			return stored, fail(409, "creation_completed")
		}
		return stored, fail(409, "creation_pending")
	}
	return stored, nil
}

// CompleteCreate binds the verified KB before making the creation usable.
func (s *Service) CompleteCreate(ctx context.Context, h http.Header, key, kb string) error {
	p, err := s.principal(ctx, h, true)
	if err != nil {
		return err
	}
	op, err := s.Store.CreationByKey(ctx, p.Tenant.ID, p.User.ID, hashKey(key))
	if err != nil {
		return err
	}
	if op.ID == "" || op.State != "pending" {
		return fail(409, "creation_missing")
	}
	native, err := s.knowledgeBase(ctx, h, p.Tenant.ID, kb)
	if err != nil {
		return err
	}
	if native.CreatorID != op.ActorUserID || contains(op.BaselineIDs, kb) {
		return fail(409, "creator_mismatch")
	}
	if _, err = s.ensure(ctx, native, true); err != nil {
		return err
	}
	return s.Store.CompleteCreation(WithAuditActor(ctx, p.User.ID), p.Tenant.ID, op.ID, kb)
}
func (s *Service) resolveCreation(ctx context.Context, h http.Header, id, kb string) (CreateOperation, error) {
	var result CreateOperation
	err := s.locked(ctx, h, kb, func() error {
		p, err := s.principal(ctx, h, true)
		if err != nil {
			return err
		}
		if !isManager(nativeRole(p)) {
			return fail(403, "admin_required")
		}
		op, err := s.Store.Creation(ctx, p.Tenant.ID, id)
		if err != nil {
			return err
		}
		if op.State != "pending" {
			return fail(409, "creation_completed")
		}
		native, err := s.knowledgeBase(ctx, h, p.Tenant.ID, kb)
		if err != nil {
			return err
		}
		if native.CreatorID != op.ActorUserID || contains(op.BaselineIDs, kb) {
			return fail(409, "creator_mismatch")
		}
		// Recovery never revives a creator whose membership was revoked while
		// the native outcome was unknown; restore-creator is a separate action.
		if _, err = s.ensure(ctx, native, false); err != nil {
			return err
		}
		if err = s.Store.CompleteCreation(WithAuditActor(ctx, p.User.ID), p.Tenant.ID, id, kb); err != nil {
			return err
		}
		result, err = s.Store.Creation(ctx, p.Tenant.ID, id)
		return err
	})
	return result, err
}
func (s *Service) creationHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.principal(r.Context(), r.Header, true)
	if err != nil {
		return nil, err
	}
	if !isManager(nativeRole(p)) {
		return nil, fail(403, "admin_required")
	}
	if r.Method == http.MethodGet && r.URL.Path == creationPrefix {
		return s.Store.Creations(r.Context(), p.Tenant.ID)
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, creationPrefix+"/"), "/")
	if r.Method != http.MethodPost || len(parts) != 2 || !validID(parts[0]) || parts[1] != "resolve" {
		return nil, fail(404, "not_found")
	}
	var input struct {
		KBID string `json:"kb_id"`
	}
	if err = decode(w, r, &input); err != nil {
		return nil, err
	}
	if !validID(input.KBID) {
		return nil, fail(400, "request_invalid")
	}
	return s.resolveCreation(r.Context(), r.Header, parts[0], input.KBID)
}

const creationColumns = `id,tenant_id,actor_user_id,key_hash,request_hash,state,kb_id,request_id,created_at,baseline_kb_ids`

type scanner interface{ Scan(...any) error }

func scanCreation(row scanner) (CreateOperation, error) {
	var op CreateOperation
	var baseline []byte
	err := row.Scan(&op.ID, &op.TenantID, &op.ActorUserID, &op.KeyHash, &op.RequestHash, &op.State, &op.KBID, &op.RequestID, &op.CreatedAt, &baseline)
	if errors.Is(err, sql.ErrNoRows) {
		return op, ErrNotFound
	}
	if err == nil && (json.Unmarshal(baseline, &op.BaselineIDs) != nil || op.BaselineIDs == nil) {
		return op, fail(503, "creation_inventory_invalid")
	}
	return op, err
}
func (r *Repository) BeginCreation(ctx context.Context, op CreateOperation) (CreateOperation, bool, error) {
	if op.BaselineIDs == nil {
		return op, false, fail(503, "creation_inventory_invalid")
	}
	baseline, err := json.Marshal(op.BaselineIDs)
	if err != nil {
		return op, false, err
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO mindcreek.kb_creation_operations(id,tenant_id,actor_user_id,key_hash,request_hash,request_id,baseline_kb_ids) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,actor_user_id,key_hash) DO NOTHING`, op.ID, op.TenantID, op.ActorUserID, op.KeyHash, op.RequestHash, op.RequestID, baseline)
	if err != nil {
		return op, false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return op, false, err
	}
	stored, err := scanCreation(r.db.QueryRowContext(ctx, `SELECT `+creationColumns+` FROM mindcreek.kb_creation_operations WHERE tenant_id=$1 AND actor_user_id=$2 AND key_hash=$3`, op.TenantID, op.ActorUserID, op.KeyHash))
	return stored, count == 1, err
}

// An unresolved operation may have produced any new KB by that creator that
// was not present before the native call. Completed bindings are exempt even
// when another operation for the same creator remains pending.
func (r *Repository) CreationPending(ctx context.Context, tenant uint64, creator, kb string) (bool, error) {
	var pending bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM mindcreek.kb_creation_operations WHERE tenant_id=$1 AND actor_user_id=$2 AND state='pending' AND NOT (baseline_kb_ids ? $3)) AND NOT EXISTS(SELECT 1 FROM mindcreek.kb_creation_operations WHERE tenant_id=$1 AND kb_id=$3 AND state='completed')`, tenant, creator, kb).Scan(&pending)
	return pending, err
}
func (r *Repository) Creation(ctx context.Context, tenant uint64, id string) (CreateOperation, error) {
	return scanCreation(r.db.QueryRowContext(ctx, `SELECT `+creationColumns+` FROM mindcreek.kb_creation_operations WHERE tenant_id=$1 AND id=$2`, tenant, id))
}
func (r *Repository) CreationByKey(ctx context.Context, tenant uint64, user, keyHash string) (CreateOperation, error) {
	return scanCreation(r.db.QueryRowContext(ctx, `SELECT `+creationColumns+` FROM mindcreek.kb_creation_operations WHERE tenant_id=$1 AND actor_user_id=$2 AND key_hash=$3`, tenant, user, keyHash))
}
func (r *Repository) Creations(ctx context.Context, tenant uint64) ([]CreateOperation, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+creationColumns+` FROM mindcreek.kb_creation_operations WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 1000`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CreateOperation{}
	for rows.Next() {
		op, err := scanCreation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, rows.Err()
}
func (r *Repository) CompleteCreation(ctx context.Context, tenant uint64, id, kb string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE mindcreek.kb_creation_operations SET state='completed',kb_id=$3,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND state='pending'`, tenant, id, kb)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fail(409, "creation_completed")
	}
	var revision uint64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM mindcreek.kb_policies WHERE tenant_id=$1 AND kb_id=$2 AND deleted=false`, tenant, kb).Scan(&revision); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, Event{TenantID: tenant, KBID: kb, ActorUserID: auditActor(ctx), Action: "creation.complete", Target: id, Revision: revision, Before: []byte(`{"state":"pending"}`), After: []byte(`{"state":"completed"}`), Outcome: "success", RequestID: diagnostics.RequestID(ctx)}); err != nil {
		return err
	}
	return tx.Commit()
}
