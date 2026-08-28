# Phase 3 Implementation Plan

| Field | Value |
|---|---|
| Status | Complete; Gates A–D passed |
| Date | 2026-08-27 |
| Upstream baseline | Unmodified WeKnora v0.7.2 |
| Outcome | Discoverable internal publications, organization-public read access, and live subscriptions |

## Boundaries and invariants

Phase 3 adds product-owned publication, audience, catalog, subscription, revision, and activity behavior. Personal Notes remain private and cannot be published. Publications reference the publisher's existing RAG KB; they never copy documents, chunks, Wiki data, or embeddings. `organization_public` means every authenticated active internal user in the configured organization audience can read, but it does not grant edit, download, publication, grant, ownership, or deletion rights.

The WeKnora submodule remains unmodified. Current identity proves an active user and workspace. The initial organization audience is the authenticated MindCreek deployment; selected-workspace audiences are evaluated against the principal's active workspace ID. Cross-workspace upstream delegation must fail closed unless the adapter can obtain authorized content from the source workspace.

## Task list

### A. Domain foundation

- [x] **P3-01 — Publication and audience contract.** Define publication states, access modes, audience evaluation, metadata limits, and typed errors.
- [x] **P3-02 — Phase 3 schema.** Add publication, subscription, content-revision, and activity tables with uniqueness, state, concurrency, and rollback constraints.
- [x] **P3-03 — Publication repository and service.** Implement owner-only publish/update/unpublish with Personal Notes denial and optimistic row versions.
- [x] **P3-04 — Subscription repository and service.** Implement retry-safe subscribe/unsubscribe/mark-seen and prevent owner self-subscription.
- [x] **P3-05 — Publication-aware authorization.** Add organization-public and active-subscriber read decisions without elevating edit permissions.
- [x] **P3-06 — Revision and activity service.** Maintain monotonic content revisions and sanitized activity records for update badges.

**Gate A — Passed on 2026-08-27:** Product-owned live-reference publication and subscription state, monotonic revisions, audience rules, and deterministic read-only authorization passed domain, repository, and eight-migration lifecycle tests. See [the acceptance record](PHASE3_GATE_A.md).

### B. APIs and enforcement

- [x] **P3-07 — Publication APIs.** Add owner-only create/update/unpublish endpoints and non-disclosing failures.
- [x] **P3-08 — Discover catalog.** Add audience-filtered search, tags, owner, access-mode, update-time filters, and pagination.
- [x] **P3-09 — Subscription APIs.** Add list, subscribe, unsubscribe, and mark-seen endpoints with typed idempotent responses.
- [x] **P3-10 — Organization-public enforcement.** Permit read-only access through every Phase 2 read route while retaining source-download denial.
- [x] **P3-11 — Subscriber enforcement.** Activate Viewer-like rendered/search/chat/citation access only for eligible active subscriptions.
- [x] **P3-12 — Inactivation lifecycle.** Unpublish and audience loss immediately remove derived access and invalidate old sessions/citations.
- [x] **P3-13 — Authorized library expansion.** Add `subscribed` and safe `all` views without adding unfollowed public KBs to default scope.
- [x] **P3-14 — Audit and negative matrix.** Cover publication/subscription lifecycle, wrong audience, owner, peer, revocation, stale revision, and Personal Notes.

**Gate B — Passed on 2026-08-27:** Publication, catalog, subscriber access, organization-public access, source-download denial, revision badges, audience loss, unpublish, and sanitized audit behavior passed the synthetic live matrix. See [the acceptance record](PHASE3_GATE_B.md).

### C. Product UI

- [x] **P3-15 — Discover and Subscribed views.** Add catalog search/filter and followed-publication navigation.
- [x] **P3-16 — Publish controls.** Add owner-only publish/update/unpublish UI with audience and access-mode controls.
- [x] **P3-17 — Update badges.** Display current/last-seen revisions and mark-seen behavior from server state.

**Gate C — Passed on 2026-08-27:** Discover, Subscribed, publication controls, and server-backed update badges passed overlay assertions, MindCreek contracts, upstream frontend tests, TypeScript checking, and the production build. See [the acceptance record](PHASE3_GATE_C.md).

### D. Release and compatibility

- [x] **P3-18 — Regression and release contract.** Run inherited gates, migrations, policy/UI tests, clean-copy deployment, and candidate-upstream contracts; publish the Phase 3 release configuration and image tag.

**Gate D — Passed on 2026-08-27:** MindCreek `0.4.0-phase3`, the inherited regression, production UI build, live migration/policy matrix, candidate-upstream contract, and reconstructed clean checkout passed with an empty upstream patch ledger. See [the acceptance record](PHASE3_GATE_D.md).

## Current implementation

P3-01 through P3-18 are implemented and Gates A–D are passed. Phase 4 can now add the unified authorized agent scope and hosted MCP facade without changing publication or subscription semantics.
