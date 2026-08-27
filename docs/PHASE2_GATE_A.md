# Phase 2 Gate A — Access-Model Foundation

| Field | Result |
|---|---|
| Status | Passed on 2026-08-27 |
| Upstream | Unmodified WeKnora v0.7.2 |
| Product migrations | Six; empty/repeat/down/up pass |
| Enabled grant subject | User |
| Roles | Owner, Editor, Viewer, None |

## Implemented foundation

MindCreek stores explicit access grants in its own `mindcreek.kb_access_grants` table. The schema provides one active grant per KB and subject, optional expiry, revocation history, Viewer/Editor validation, optimistic revisions, and an audit correlation field. It has no foreign key or write dependency on upstream tables. Group and workspace subject values are reserved in the schema but rejected by the service until a trusted directory resolver exists.

The ownership resolver reconciles a product KB profile with WeKnora's `creator_id` and tenant through the adapter. Conflicting ownership, unavailable dependencies, and ownerless legacy KBs fail closed; ownerless KBs require an explicit future adoption flow.

The grant service permits only the exact owner in the exact tenant to create, list, update, or revoke grants. It rejects Personal Notes, self-grants, unsupported subjects, stale revisions, invalid expiry, and conflicting grants. Equivalent retries—including a concurrent uniqueness winner—are idempotent.

The authorization decision service resolves Owner, Editor, Viewer, or None. Viewer permits discovery and read; Editor additionally permits content editing; only Owner permits configuration, grant management, and deletion. Cross-tenant, expired, revoked, ungranted, and Personal Notes peer access resolve to None. Dependency failures deny access.

## Gate boundary

Gate A is an internal policy foundation only. It does not add sharing endpoints, route enforcement, authorized list views, or UI controls. Those begin at P2-07 and must pass Gate B before sharing is usable.

## Reproduce

```sh
make phase2-check
make phase1-gateway-build-offline
make phase2-gate-a
```

The live migration probe creates and removes a temporary database and writes its compact report under `.local/`. It verifies that the upstream `public` schema is unchanged.
