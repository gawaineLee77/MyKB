# Phase 2 Sharing-Model Map

| Field | Value |
|---|---|
| Task | P2-01 |
| Status | Complete |
| Verified | 2026-08-27 |
| Baseline | WeKnora v0.7.2 (`3d5d8bfcdfeeea266b292b71cea616847af28d0f`) |

## Upstream behavior

WeKnora stores KB ownership context on `knowledge_bases.tenant_id` and `knowledge_bases.creator_id`, but its normal list repository selects every non-temporary KB in the caller's tenant. Workspace membership therefore remains broader than MindCreek's private-by-default read rule.

The upstream `kb_shares` table shares one KB with an `organization_id`. Organization membership is tenant/workspace based, not an individual-user grant. Its effective permission is capped by the share permission, the recipient tenant's organization role, and the caller's tenant role. Active rows are unique by KB and organization and use soft deletion. Upstream exposes Viewer/Editor vocabulary, cross-tenant source resolution, share APIs, and `kb.share_*` audit actions.

## Target mapping

| MindCreek concept | Upstream seam | Phase 2 decision |
|---|---|---|
| KB owner | `knowledge_bases.creator_id` plus `tenant_id` | Reuse through the versioned adapter; product profiles remain authoritative for MindCreek-created KBs. |
| Viewer/Editor vocabulary | `OrgMemberRole` and share API values | Normalize the names only; do not import upstream authorization decisions. |
| Individual user grant | No equivalent | Store in `mindcreek.kb_access_grants`. |
| Group/workspace grant | `kb_shares` targets organizations of tenants | Reserve as a future subject resolver; do not enable implicitly in Phase 2. |
| Grant lifecycle | Soft delete and timestamps | Add explicit expiry, revocation, and optimistic revision in the product schema. |
| Source tenant for retrieval | `source_tenant_id` and KB metadata | Resolve behind the adapter after the product policy grants access. |
| Audit | `kb.share_added`, changed, and removed | Emit product audit records with actor, old/new value, request ID, and outcome. |
| Shared-agent visibility | Read fallback through an organization-shared agent | Do not treat as a Phase 2 KB grant; Phase 4 will apply the unified scope resolver. |

## Gaps that require product ownership

- Upstream organization sharing cannot express a direct `user` subject.
- Tenant-wide KB lists reveal peer-created KBs before an explicit grant exists.
- Upstream source-tenant administrators may manage organization shares; MindCreek grant management is KB-owner only.
- Upstream accepts an administrator role internally, while product grants are only `viewer` or `editor`.
- There is no product-required `expires_at`, `revoked_at`, or concurrency revision.
- Personal Notes must remain unshareable even if an upstream share endpoint is called directly.
- POST search, chat, and citation resolution are read operations and cannot be classified by HTTP method alone.

## Boundary decision

Phase 2 will add a companion grant model outside the submodule:

```text
mindcreek.kb_access_grants
- id
- knowledge_base_id
- subject_type: user | group | workspace
- subject_id
- permission: viewer | editor
- granted_by
- created_at, updated_at, expires_at, revoked_at
- revision
```

The first enabled resolver is `subject_type=user`. Group and workspace subjects remain rejected until a trusted membership source is selected. Existing upstream organization shares are compatibility candidates, not implicit MindCreek grants.

The gateway will resolve ownership first, then active product grants, before proxying any KB-scoped request. Missing profiles may use upstream `creator_id`; a legacy KB with no reliable creator requires explicit owner adoption and otherwise fails closed. Ordinary workspace membership and upstream administrator status do not automatically disclose private content. A future administrative override must be explicit and audited.

## Reused files and seams

- `upstream/weknora/internal/types/knowledgebase.go`
- `upstream/weknora/migrations/versioned/000012_organizations.up.sql`
- `upstream/weknora/internal/application/repository/kbshare.go`
- `upstream/weknora/internal/application/service/kbshare.go`
- `upstream/weknora/internal/middleware/kb_access.go`
- `upstream/weknora/internal/types/audit_log.go`

No upstream source or schema is modified by this decision.
