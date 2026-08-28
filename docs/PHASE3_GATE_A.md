# Phase 3 Gate A — Publication Foundation

| Field | Result |
|---|---|
| Status | Passed on 2026-08-27 |
| Scope | P3-01 through P3-06 |
| Upstream | Unmodified WeKnora v0.7.2 |
| Product migrations | Eight total; migration 8 is Phase 3 |

## Implemented foundation

MindCreek owns publication, subscription, content-revision, and activity state in migration `000008_phase3_publications`. Publications are live references to existing WeKnora RAG KBs; no documents, chunks, Wiki data, or embeddings are copied. Unique constraints permit one publication per KB and one subscription per publication/user, while row versions protect publication changes.

The publication service accepts `subscriber` or `organization_public` access and `organization` or selected-workspace audiences. Only the exact owner can publish, update, or unpublish. Personal Notes always reject publication. Metadata is bounded and normalized, and audience changes inactivate subscriptions that are no longer eligible.

Subscription operations are idempotent and prevent owner self-subscription. Content changes maintain a monotonic revision and a sanitized activity event. The authorization decision service derives Viewer-like read access from an eligible active subscription or an organization-public publication, without granting editing, download, publication, grant management, ownership, or deletion.

## Reproduce

```sh
make phase3-gate-a
```

This runs inherited checks, focused domain tests, the candidate-upstream contract, and the eight-migration empty/repeat/down/up lifecycle.
