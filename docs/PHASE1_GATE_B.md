# Phase 1 Gate B — Personal Notes Security

| Field | Result |
|---|---|
| Status | Passed on 2026-08-26 |
| Upstream | Unmodified WeKnora v0.7.2 |
| Gateway tests | All packages pass |
| Live matrix | 24 negative authorization rows pass |
| Upstream patch count | 0 |

## Implemented controls

The gateway owns a separate `mindcreek` PostgreSQL schema. Embedded, checksummed migrations run under a PostgreSQL advisory lock and support `up`, `down`, and `status`. The `kb_profiles` table records the upstream KB ID, workspace, owner, product mode, schema version, access policy, and timestamps.

Every KB-policy-controlled request resolves the current user and active workspace through WeKnora's authenticated `/api/v1/auth/me` contract. Client-supplied MindCreek identity headers are removed. Missing, invalid, malformed, or cross-workspace identity fails closed.

For a `personal_notes` profile, only the exact owner in the recorded workspace may read or write. Workspace Owner/Admin roles do not bypass this rule. Share and publish operations are always denied. A non-owner receives the same `404 resource.not_found` response used for an absent resource.

## Protected route matrix

| Surface | Enforcement |
|---|---|
| KB lists, shared lists, organization shares, favorites | Remove non-owned Note Spaces and recompute totals |
| KB, manual source, FAQ, tag, Wiki | Resolve direct KB or parent knowledge ID before proxying |
| Chunks, preview, download, images, KB files | Resolve parent KB before returning derived content |
| Hybrid search and knowledge search | Inspect path, query, and JSON retrieval scopes |
| Sessions and messages | Validate the authenticated session owner |
| Knowledge chat, agent chat, agent configuration | Validate session, agent, KB, and knowledge selections before retrieval |
| Copy, move, and batch operations | Inspect every source, target, KB, and knowledge reference |

Raw `/files`, `/r/:token`, and presigned-file routes remain disabled. The v0.7.2 move, copy, and FAQ progress endpoints expose only opaque task IDs; the gateway returns `404` until a product-owned task-to-KB mapping can prove access.

## Reproduce the gate

Run against the isolated Phase 1 stack:

```bash
make phase1-gateway-build-offline
make phase1-up
make phase1-gate-b
make phase1-check
```

The migration probe uses and removes a temporary database. The authorization probe creates synthetic local Alice/Bob users and resources and writes compact reports under `.local/`.

Gate B made Personal Notes eligible for product work. P1-14 subsequently enabled the capability through atomic, idempotent product creation; the stock WeKnora KB form still cannot create a profiled Note Space.
