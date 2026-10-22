package workspaceadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// KnowledgeStore is separate from workspace-member delegation: authorizing
// member administration never implicitly authorizes content execution.
type KnowledgeStore interface {
	ExecutionDelegation(context.Context, uint64) (Delegation, error)
	SaveExecutionDelegation(context.Context, Delegation) error
	DeleteExecutionDelegation(context.Context, uint64) error
	ExecutionKeys(context.Context, uint64) ([]ExecutionKey, error)
	SaveExecutionKey(context.Context, ExecutionKey) error
	DeleteExecutionKeys(context.Context, uint64) error
	MachinePolicy(context.Context, uint64, uint64) (MachinePolicy, error)
	SaveMachinePolicy(context.Context, MachinePolicy) error
	DeleteMachinePolicy(context.Context, uint64, uint64) error
}
type ExecutionKey struct {
	TenantID, KeyID uint64
	KBID            string
	Cipher          []byte
}
type MachinePolicy struct {
	TenantID          uint64    `json:"-"`
	KeyID             uint64    `json:"-"`
	Fingerprint       string    `json:"-"`
	KnowledgeBaseIDs  []string  `json:"knowledge_base_ids"`
	Operations        []string  `json:"knowledge_operations"`
	AllKnowledgeBases bool      `json:"all_knowledge_bases"`
	UpdatedAt         time.Time `json:"-"`
}
type KnowledgeRepository struct{ DB *sql.DB }

func (r *KnowledgeRepository) ExecutionDelegation(ctx context.Context, tenant uint64) (Delegation, error) {
	var d Delegation
	err := r.DB.QueryRowContext(ctx, `SELECT tenant_id,owner_user_id,session_cipher,updated_at FROM mindcreek.kb_execution_delegations WHERE tenant_id=$1`, tenant).Scan(&d.TenantID, &d.OwnerUserID, &d.SessionCipher, &d.UpdatedAt)
	return d, notFound(err)
}
func (r *KnowledgeRepository) SaveExecutionDelegation(ctx context.Context, d Delegation) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO mindcreek.kb_execution_delegations(tenant_id,owner_user_id,session_cipher) VALUES($1,$2,$3) ON CONFLICT(tenant_id) DO UPDATE SET owner_user_id=EXCLUDED.owner_user_id,session_cipher=EXCLUDED.session_cipher,updated_at=now()`, d.TenantID, d.OwnerUserID, d.SessionCipher)
	return err
}
func (r *KnowledgeRepository) DeleteExecutionDelegation(ctx context.Context, tenant uint64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM mindcreek.kb_execution_delegations WHERE tenant_id=$1`, tenant)
	return err
}
func (r *KnowledgeRepository) ExecutionKeys(ctx context.Context, tenant uint64) ([]ExecutionKey, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT tenant_id,kb_id,key_id,key_cipher FROM mindcreek.kb_execution_keys WHERE tenant_id=$1 ORDER BY kb_id`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExecutionKey{}
	for rows.Next() {
		var k ExecutionKey
		if err := rows.Scan(&k.TenantID, &k.KBID, &k.KeyID, &k.Cipher); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
func (r *KnowledgeRepository) SaveExecutionKey(ctx context.Context, k ExecutionKey) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO mindcreek.kb_execution_keys(tenant_id,kb_id,key_id,key_cipher) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,kb_id) DO UPDATE SET key_id=EXCLUDED.key_id,key_cipher=EXCLUDED.key_cipher`, k.TenantID, k.KBID, k.KeyID, k.Cipher)
	return err
}
func (r *KnowledgeRepository) DeleteExecutionKeys(ctx context.Context, tenant uint64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM mindcreek.kb_execution_keys WHERE tenant_id=$1`, tenant)
	return err
}
func (r *KnowledgeRepository) MachinePolicy(ctx context.Context, tenant, key uint64) (MachinePolicy, error) {
	var p MachinePolicy
	var ids, ops []byte
	err := r.DB.QueryRowContext(ctx, `SELECT tenant_id,key_id,fingerprint,knowledge_base_ids,operations,all_knowledge_bases,updated_at FROM mindcreek.kb_machine_permissions WHERE tenant_id=$1 AND key_id=$2`, tenant, key).Scan(&p.TenantID, &p.KeyID, &p.Fingerprint, &ids, &ops, &p.AllKnowledgeBases, &p.UpdatedAt)
	if err != nil {
		return p, notFound(err)
	}
	if err = json.Unmarshal(ids, &p.KnowledgeBaseIDs); err != nil {
		return p, err
	}
	err = json.Unmarshal(ops, &p.Operations)
	return p, err
}
func (r *KnowledgeRepository) SaveMachinePolicy(ctx context.Context, p MachinePolicy) error {
	if p.KnowledgeBaseIDs == nil {
		p.KnowledgeBaseIDs = []string{}
	}
	if p.Operations == nil {
		p.Operations = []string{}
	}
	ids, _ := json.Marshal(p.KnowledgeBaseIDs)
	ops, _ := json.Marshal(p.Operations)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO mindcreek.kb_machine_permissions(tenant_id,key_id,fingerprint,knowledge_base_ids,operations,all_knowledge_bases) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,key_id) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,knowledge_base_ids=EXCLUDED.knowledge_base_ids,operations=EXCLUDED.operations,all_knowledge_bases=EXCLUDED.all_knowledge_bases,updated_at=now()`, p.TenantID, p.KeyID, p.Fingerprint, ids, ops, p.AllKnowledgeBases)
	return err
}
func (r *KnowledgeRepository) DeleteMachinePolicy(ctx context.Context, tenant, key uint64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM mindcreek.kb_machine_permissions WHERE tenant_id=$1 AND key_id=$2`, tenant, key)
	return err
}
