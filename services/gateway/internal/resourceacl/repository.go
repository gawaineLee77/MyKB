package resourceacl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Native reads deliberately name the public schema. These columns and their
// write provenance were audited against PinnedUpstream; no upstream writes,
// trigger, migration, internal Go imports, or editable-body ownership claims
// are used. Review this adapter when changing that pin.
const resourceColumns = `id,handle,tenant_id,physical_path,location_hash,kind,original_name`
const activeResources = ` FROM public.resources WHERE tenant_id=$1 AND state='active' AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at>now())`

func loadResource(ctx context.Context, q querier, tenant uint64, reference string) (Resource, error) {
	out := Resource{TenantID: tenant, Owners: []Owner{}}
	if tenant == 0 {
		return out, ErrInvalidReference
	}
	handle, isHandle := handleOf(reference)
	query := `SELECT ` + resourceColumns + activeResources
	var lookup string
	if isHandle {
		query += ` AND handle=$2`
		lookup = handle
	} else {
		if !validPhysical(reference, tenant) {
			return out, ErrInvalidReference
		}
		query += ` AND location_hash=$2 AND physical_path=$3`
		lookup = digest(reference)
	}
	var locationHash, kind, originalName string
	var err error
	if isHandle {
		err = q.QueryRowContext(ctx, query, tenant, lookup).Scan(&out.ID, &handle, &out.TenantID, &out.PhysicalPath, &locationHash, &kind, &originalName)
	} else {
		err = q.QueryRowContext(ctx, query, tenant, lookup, reference).Scan(&out.ID, &handle, &out.TenantID, &out.PhysicalPath, &locationHash, &kind, &originalName)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("%w: resource registry", ErrUnavailable)
	}
	if out.TenantID != tenant || !handlePattern.MatchString(handle) || out.ID == "" ||
		kind != "image" || !validPhysical(out.PhysicalPath, tenant) || locationHash != digest(out.PhysicalPath) {
		return Resource{}, ErrNotFound
	}
	out.Reference = "resource://" + handle
	out.Fingerprint = fingerprint(tenant, out.ID, out.PhysicalPath)
	out.SessionScoped = strings.HasPrefix(originalName, "chat-images/")
	return out, nil
}

// This is denial-only evidence. Even an editable attachment lifecycle binding
// may exclude an object from the broad historical KB manager exception, but
// can never grant access. Include soft-deleted messages so deleting a private
// conversation does not make its leftover images workspace-readable.
func sessionScoped(ctx context.Context, q querier, resource Resource) (bool, error) {
	if resource.SessionScoped {
		return true, nil
	}
	var scoped bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM public.resource_bindings b
WHERE b.tenant_id=$1 AND b.resource_id=$2 AND b.owner_type IN ('message','temporary_document')
) OR EXISTS(
SELECT 1 FROM public.messages m JOIN public.sessions s ON s.id=m.session_id
WHERE s.tenant_id=$1 AND EXISTS(
  SELECT 1 FROM jsonb_array_elements(
    CASE WHEN jsonb_typeof(m.images)='array' THEN m.images ELSE '[]'::jsonb END
  ) AS image WHERE image->>'url' IN ($3,$4)))`,
		resource.TenantID, resource.ID, resource.Reference, resource.PhysicalPath).Scan(&scoped)
	if err != nil {
		return false, ErrUnavailable
	}
	return scoped, nil
}

// A multimodal image subspan is created from newly persisted bytes by the
// worker, not from arbitrary Markdown handles. The same-knowledge/attempt
// parent join is essential; an unrelated span with a similar name is not
// provenance. Attachments in resource_bindings are intentionally excluded:
// native manual-document saving can create those from pasted handles.
const ownersSQL = `
SELECT DISTINCT k.knowledge_base_id,k.id,'processing_span' AS provenance
FROM public.knowledge_processing_spans s
JOIN public.knowledge_processing_spans p
  ON p.knowledge_id=s.knowledge_id AND p.attempt=s.attempt AND p.span_id=s.parent_span_id
JOIN public.knowledges k ON k.id=s.knowledge_id
JOIN public.knowledge_bases kb ON kb.id=k.knowledge_base_id AND kb.tenant_id=k.tenant_id
WHERE k.tenant_id=$1 AND k.deleted_at IS NULL AND k.parse_status<>'deleting' AND kb.deleted_at IS NULL
  AND s.attempt>0 AND s.kind='generation' AND s.name ~ '^multimodal[.]image\[[0-9]+\]$'
  AND p.kind='stage' AND p.name='multimodal' AND s.input->>'image_url' IN ($2,$3)
UNION
SELECT k.knowledge_base_id,k.id,'operator_review' AS provenance
FROM mindcreek.kb_resource_bindings b
JOIN public.knowledges k ON k.id=b.knowledge_id AND k.tenant_id=b.tenant_id
JOIN public.knowledge_bases kb ON kb.id=k.knowledge_base_id AND kb.tenant_id=k.tenant_id
WHERE b.tenant_id=$1 AND b.resource_id=$4 AND b.resource_fingerprint=$5
  AND k.deleted_at IS NULL AND k.parse_status<>'deleting' AND kb.deleted_at IS NULL
ORDER BY 1,2,3`

func owners(ctx context.Context, q querier, resource Resource) ([]Owner, error) {
	rows, err := q.QueryContext(ctx, ownersSQL, resource.TenantID, resource.Reference, resource.PhysicalPath, resource.ID, resource.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("%w: resource provenance", ErrUnavailable)
	}
	defer rows.Close()
	out := []Owner{}
	for rows.Next() {
		var owner Owner
		if err := rows.Scan(&owner.KBID, &owner.KnowledgeID, &owner.Provenance); err != nil {
			return nil, ErrUnavailable
		}
		if owner.KBID == "" || owner.KnowledgeID == "" {
			return nil, ErrUnavailable
		}
		out = append(out, owner)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

func (r *Repository) Resolve(ctx context.Context, tenant uint64, reference string) (Resource, error) {
	if r == nil || r.db == nil {
		return Resource{}, ErrUnavailable
	}
	resource, err := loadResource(ctx, r.db, tenant, reference)
	if err != nil {
		return Resource{}, err
	}
	resource.Owners, err = owners(ctx, r.db, resource)
	if err != nil {
		return Resource{}, err
	}
	resource.SessionScoped, err = sessionScoped(ctx, r.db, resource)
	return resource, err
}

// Approve is for the local installation operator's reviewed manifest only; it
// must never be exposed as a normal knowledge-editor endpoint. An exact
// fingerprint makes approving a stale/replaced object fail instead of silently
// granting it access. Existing approval evidence is immutable and idempotent.
func (r *Repository) Approve(ctx context.Context, tenant uint64, reference, knowledgeID, expectedFingerprint, reason string) error {
	reason = strings.TrimSpace(reason)
	if knowledgeID == "" || expectedFingerprint == "" || reason == "" || len(reason) > 2000 || !utf8.ValidString(reason) {
		return ErrInvalidApproval
	}
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback()
	resource, err := loadResource(ctx, tx, tenant, reference)
	if err != nil {
		return err
	}
	if resource.Fingerprint != expectedFingerprint {
		return ErrFingerprintMismatch
	}
	var kb string
	err = tx.QueryRowContext(ctx, `SELECT k.knowledge_base_id FROM public.knowledges k
JOIN public.knowledge_bases kb ON kb.id=k.knowledge_base_id AND kb.tenant_id=k.tenant_id
WHERE k.tenant_id=$1 AND k.id=$2 AND k.deleted_at IS NULL AND k.parse_status<>'deleting' AND kb.deleted_at IS NULL`,
		tenant, knowledgeID).Scan(&kb)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO mindcreek.kb_resource_bindings
(tenant_id,resource_id,resource_fingerprint,knowledge_id,reason)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(tenant_id,resource_id,resource_fingerprint,knowledge_id) DO NOTHING`,
		tenant, resource.ID, resource.Fingerprint, knowledgeID, reason); err != nil {
		return ErrUnavailable
	}
	if err := tx.Commit(); err != nil {
		return ErrUnavailable
	}
	return nil
}

// Audit is bounded, read-only and intentionally does not suggest an owner based
// on editable content. The operator must independently review each mapping.
// Objects outside the KB image exports namespace are not candidates.
func (r *Repository) Audit(ctx context.Context, tenant uint64, limit int) (AuditReport, error) {
	return r.AuditPage(ctx, tenant, limit, "")
}

// AuditPage uses a stable resource-ID cursor; a page can contain no eligible
// exports images and still have NextCursor if it scanned source-file uploads.
func (r *Repository) AuditPage(ctx context.Context, tenant uint64, limit int, afterID string) (AuditReport, error) {
	report := AuditReport{TenantID: tenant, Items: []AuditItem{}}
	if r == nil || r.db == nil {
		return report, ErrUnavailable
	}
	if tenant == 0 || limit < 1 || limit > 1000 || len(afterID) > 64 {
		return report, ErrInvalidReference
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*)`+activeResources+` AND kind='image'`, tenant).Scan(&report.Total); err != nil {
		return report, ErrUnavailable
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,handle`+activeResources+` AND kind='image' AND id>$3 ORDER BY id LIMIT $2`, tenant, limit+1, afterID)
	if err != nil {
		return report, ErrUnavailable
	}
	var references []string
	var ids []string
	for rows.Next() {
		var id, handle string
		if err := rows.Scan(&id, &handle); err != nil {
			rows.Close()
			return report, ErrUnavailable
		}
		references = append(references, "resource://"+handle)
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, ErrUnavailable
	}
	report.Truncated = len(references) > limit
	if report.Truncated {
		report.NextCursor = ids[limit-1]
		references = references[:limit]
	}
	for _, reference := range references {
		resource, err := r.Resolve(ctx, tenant, reference)
		if errors.Is(err, ErrNotFound) {
			// Source uploads and concurrently deleted resources aren't images
			// served by the KB proxy and cannot become approval candidates.
			continue
		}
		if err != nil {
			return report, err
		}
		item := AuditItem{Resource: resource}
		report.Scanned++
		if len(resource.Owners) > 0 {
			report.Verified++
		} else {
			report.Unverified++
			item.Reason = "no_trusted_knowledge_provenance"
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}

// MessageImage recognizes only a user-uploaded image in the specified live
// session. QA clears client URL fields before saving the bytes, and no public
// message mutation endpoint accepts Images. Caller/session authorization is a
// separate mandatory check performed before calling this method.
func (r *Repository) MessageImage(ctx context.Context, tenant uint64, sessionID, messageID, reference string) (bool, error) {
	if sessionID == "" || messageID == "" {
		return false, ErrInvalidReference
	}
	if r == nil || r.db == nil {
		return false, ErrUnavailable
	}
	resource, err := loadResource(ctx, r.db, tenant, reference)
	if err != nil {
		return false, err
	}
	var matched bool
	err = r.db.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM public.messages m JOIN public.sessions s ON s.id=m.session_id
WHERE s.id=$2 AND s.tenant_id=$1 AND s.deleted_at IS NULL
  AND m.id=$3 AND m.session_id=$2 AND m.role='user' AND m.deleted_at IS NULL
  AND EXISTS(SELECT 1 FROM jsonb_array_elements(
    CASE WHEN jsonb_typeof(m.images)='array' THEN m.images ELSE '[]'::jsonb END
  ) AS image WHERE image->>'url' IN ($4,$5)))`,
		tenant, sessionID, messageID, resource.Reference, resource.PhysicalPath).Scan(&matched)
	if err != nil {
		return false, ErrUnavailable
	}
	return matched, nil
}
