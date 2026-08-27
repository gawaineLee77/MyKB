# Phase 2 Implementation Plan

| Field | Value |
|---|---|
| Status | Complete; Gates A–D passed |
| Date | 2026-08-27 |
| Upstream baseline | Unmodified WeKnora v0.7.2 |
| Delivery rule | Implement and verify one numbered task at a time |

## Objective and boundaries

Phase 2 makes Document RAG knowledge bases private by default and shareable through explicit Viewer or Editor grants. Personal Notes remain owner-only and cannot be shared. Publication, organization-public access, catalog, and subscriptions remain Phase 3; MCP scope remains Phase 4; corporate OAuth 2.0 and closed registration remain Phase 5.

No task may modify `upstream/weknora`. Product grants, policy, APIs, audit records, and UI remain in the MindCreek gateway, schema, and frontend overlay. Missing ownership or authorization data fails closed.

## Delivery rules

- Complete one task and its acceptance check before starting the next.
- Treat ordinary workspace membership as insufficient to read a private KB.
- Return non-disclosing errors for unauthorized resource access.
- Keep Personal Notes sharing disabled at both UI and server layers.
- Classify operations by behavior: POST-based search/chat is read, not edit.
- Use synthetic users and content for authorization tests and screenshots.

## Task list

### A. Access-model foundation

- [x] **P2-01 — Upstream sharing-model map.** Compare WeKnora ownership, tenant listing, organization sharing, permissions, and audit behavior with the MindCreek grant model. **Accept:** the verified map records reusable seams, rejected assumptions, and the product-owned boundary. See [the sharing-model map](PHASE2_SHARING_MODEL.md).
- [x] **P2-02 — Permission and route-action inventory.** Classify every KB-scoped route as discover, read, edit-content, configure, manage-grants, or delete. **Accept:** every v0.7.2 KB route has an owner/editor/viewer decision, including POST search/chat and derived content. See [the verified route-action inventory](PHASE2_ROUTE_ACTIONS.md).
- [x] **P2-03 — Grant schema migration.** Add product-owned access grants, active uniqueness, expiry/revocation, optimistic revision, and audit correlation. **Accept:** empty/repeat/down/up migration tests pass without changing upstream tables. See [Gate A evidence](PHASE2_GATE_A.md).
- [x] **P2-04 — Ownership resolver.** Resolve product profiles and upstream `creator_id` through the adapter; require explicit adoption for ownerless legacy KBs. **Accept:** missing or conflicting ownership fails closed. See [Gate A evidence](PHASE2_GATE_A.md).
- [x] **P2-05 — Grant repository and service.** Implement owner-only create/list/update/revoke with Viewer/Editor validation and idempotency. **Accept:** repository and service tests cover uniqueness, expiry, concurrency, and Personal Notes denial. See [Gate A evidence](PHASE2_GATE_A.md).
- [x] **P2-06 — Authorization decision service.** Resolve Owner, Editor, Viewer, or None without automatic administrator content access. **Accept:** table-driven policy tests cover owner, grantee, peer, wrong tenant, expired, and revoked cases. See [Gate A evidence](PHASE2_GATE_A.md).

**Gate A — Passed on 2026-08-27:** Product-owned grants and deterministic policy decisions exist, but no sharing API, route enforcement, or UI is enabled. See [the acceptance record](PHASE2_GATE_A.md).

### B. Enforcement and APIs

- [x] **P2-07 — Private-by-default request enforcement.** Apply decisions to KB, document, chunk, FAQ, Wiki, preview, download, retrieval, chat, citation, and agent-selection paths. **Accept:** same-workspace peers cannot infer or retrieve private content by guessed IDs.
- [x] **P2-08 — Authorized list views.** Add paginated `owned` and `shared` product views and filter upstream lists before returning them. **Accept:** `My KBs` contains only owned KBs; `Shared with me` contains active grants only.
- [x] **P2-09 — Grant APIs.** Add owner-only list/create/patch/delete endpoints with typed errors and revision preconditions. **Accept:** modified clients cannot share Personal Notes or grant unsupported permissions/subjects.
- [x] **P2-10 — Viewer behavior.** Permit authorized read, search, chat, citations, and policy-approved downloads while denying every mutation. **Accept:** the live matrix proves read success and write denial.
- [x] **P2-11 — Editor behavior.** Permit content and limited configuration changes while denying grant management, publication, ownership transfer, and KB deletion. **Accept:** the live matrix covers each boundary.
- [x] **P2-12 — Revocation and expiry.** Invalidate access immediately and reauthorize old citations and sessions. **Accept:** revoked/expired users disappear from lists and cannot reuse prior URLs or chat references.
- [x] **P2-13 — Audit events.** Record grant create/update/revoke and denied high-value operations with actor, target, old/new values, request ID, and outcome. **Accept:** synthetic actions produce redacted, correlated records.
- [x] **P2-14 — Negative authorization matrix.** Cover owner, viewer, editor, peer, wrong workspace, expired/revoked grantee, and Personal Notes across all route families. **Accept:** the matrix passes through the public frontend endpoint.

**Gate B — Passed on 2026-08-27:** Private-by-default and explicit Viewer/Editor sharing are enforced end to end. See [the acceptance record](PHASE2_GATE_B.md).

### C. Product UI

- [x] **P2-15 — My KBs and Shared with me.** Add product-owned navigation and empty/loading/error states. **Accept:** each view matches the gateway result and exposes no upstream workspace-wide fallback.
- [x] **P2-16 — Sharing dialog.** Add user lookup, permission selection, update, revoke, expiry display, and concurrency feedback. **Accept:** only owners see controls; Personal Notes never show sharing actions.
- [x] **P2-17 — Permission-aware workspaces.** Render Viewer and Editor affordances from server decisions, not client assumptions. **Accept:** UI contracts, production type-check/build, and live access summaries cover owner/viewer/editor behavior; screenshots were deferred by user direction.

**Gate C — Passed on 2026-08-27:** Internal users can safely discover owned/shared KBs and manage explicit grants through MindCreek. See [the acceptance record](PHASE2_GATE_C.md).

### D. Release and compatibility

- [x] **P2-18 — Regression and upgrade contract.** Run all Phase 1 gates plus Phase 2 policy, migration, UI, and candidate-upstream adapter checks. **Accept:** clean-checkout deployment passes and the upstream patch ledger remains empty. See [Gate D evidence](PHASE2_GATE_D.md).

**Gate D — Passed on 2026-08-27:** Phase 2 has a versioned release registry and image, complete regression entry point, clean-copy deployment check, and candidate-upstream contract. See [the acceptance record](PHASE2_GATE_D.md).

## Recommended next phase

Plan Phase 3 publication, organization-public KBs, catalog discovery, and subscriptions without weakening Phase 2 private-by-default enforcement.
