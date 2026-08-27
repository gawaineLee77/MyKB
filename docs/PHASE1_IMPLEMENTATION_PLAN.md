# Phase 1 Implementation Plan

| Field | Value |
|---|---|
| Status | Active; Gates A–D complete, P1-22 next |
| Date | 2026-08-26 |
| Upstream baseline | Unmodified WeKnora v0.7.2 |
| Delivery rule | Implement and verify one numbered task at a time |

## Objective and boundaries

Phase 1 delivers a non-invasive, web-first MindCreek distribution with two enabled product modes: owner-only Personal Notes and multi-format Plain RAG. WeChat Mini Program, CLI, IM, public embed, Web search, external connectors, and hosted MCP remain absent or unreachable.

Phase 1 preserves WeKnora authentication. Corporate OAuth 2.0 and closed registration remain Phase 5 work. Phase 1 enforces owner-only policy specifically for `personal_notes`; Phase 2 extends private-by-default policy and explicit sharing to general knowledge bases.

No task may modify `upstream/weknora`. Product code will use a standalone Go 1.26 gateway, product-owned PostgreSQL migrations, a versioned REST adapter, deployment overlays, and product-owned frontend modules copied into the ephemeral UI build.

## Delivery rules

- Complete one task and its acceptance check before starting the next.
- Keep unfinished modes disabled in the server capability response.
- Keep WeKnora private; browsers must not bypass the Product Gateway.
- Add negative authorization tests before enabling Personal Notes.
- Use synthetic test content only. Keep each change small enough to review independently.

## Task list

### A. Gateway and distribution foundation

- [x] **P1-01 — Route and policy inventory.** Record upstream route families as pass-through, product-owned, disabled, or KB-policy-controlled. **Accept:** every v0.7.2 route is classified and the Personal Notes enforcement points are listed. See [the verified route-policy inventory](PHASE1_ROUTE_POLICY.md).
- [x] **P1-02 — Gateway skeleton.** Add a standalone service with configuration, structured errors, `/health`, and `/version`; no business behavior. **Accept:** unit tests and a local binary/container health check pass. See [the Phase 1 gateway guide](PHASE1_GATEWAY.md).
- [x] **P1-03 — Versioned WeKnora adapter.** Add only health, version, and current-principal calls with timeouts and error translation. **Accept:** contract tests pass against v0.7.2 and unsupported versions fail closed.
- [x] **P1-04 — Exclusive network path.** Route UI API traffic through the gateway and make the WeKnora app private in Compose. **Accept:** the browser works through the gateway and cannot reach the app port directly.
- [x] **P1-05 — Capability registry.** Serve one authoritative capability document; initially enable Plain RAG and keep Personal Notes disabled until its security gate passes. **Accept:** configuration and API tests agree on every flag.
- [x] **P1-06 — Disabled-feature enforcement.** Deny excluded route families in the gateway and omit related services/credentials. **Accept:** direct HTTP probes receive `feature.disabled`; upstream remains clean.

**Gate A — passed 2026-08-26:** Gateway is the only product API path, excluded capabilities are denied, and stock login configuration plus permitted web behavior works through the gateway. The live check covers nine disabled route probes and confirms no host binding exists for the gateway or WeKnora app.

### B. Product state and Personal Notes security

- [x] **P1-07 — Product database migration runner.** Create a separate `mindcreek` schema and migration history. **Accept:** empty install, repeat startup, and rollback/forward test pass without changing upstream tables.
- [x] **P1-08 — KB product profiles.** Add storage for upstream KB ID, owner ID, product mode, schema version, policy, and timestamps. **Accept:** repository tests cover create, lookup, uniqueness, and missing upstream resources.
- [x] **P1-09 — Principal resolution.** Resolve the authenticated upstream user and workspace at the gateway; never trust client-supplied owner identity. **Accept:** missing, invalid, and cross-workspace credentials fail closed.
- [x] **P1-10 — Personal Notes policy service.** Implement owner-only read/write, and deny share/publish operations for Note Spaces. **Accept:** table-driven owner/non-owner/admin policy tests pass.
- [x] **P1-11 — KB and source-route enforcement.** Filter Note Spaces from non-owner lists and protect KB, manual note, knowledge, FAQ, chunk, and tag routes. **Accept:** Bob cannot list or access Alice's synthetic Note Space by guessing IDs.
- [x] **P1-12 — Retrieval-route enforcement.** Restrict search, session, knowledge-chat, agent-chat, and agent KB selection before upstream retrieval. **Accept:** non-owner requests cannot retrieve note sentinels or infer Note Space existence.
- [x] **P1-13 — Derived-content enforcement.** Protect preview, download, image, export, citation, and Wiki routes. **Accept:** old or guessed references receive the same denial after access checks.

**Gate B — passed 2026-08-26:** The migration lifecycle, trusted-principal checks, owner/no-admin-bypass policy, 24-row live negative authorization matrix, ordinary-RAG control, route inventory, and upstream boundary all pass. See [the Gate B security record](PHASE1_GATE_B.md).

### C. Personal Notes user functions

- [x] **P1-14 — Mode creation API.** Add `GET /capabilities/knowledge-modes` and atomic/idempotent Note Space creation over the adapter plus product profile. **Accept:** retries create one upstream KB and one profile; partial failure reconciles safely.
- [x] **P1-15 — Small creation wizard.** Add product-owned UI selection for Personal Notes and Document RAG/Plain only. **Accept:** disabled future modes cannot be submitted, including by a modified client.
- [x] **P1-16 — Note CRUD and editor.** Wrap upstream manual Markdown create/read/update/delete and add the MindCreek Notes view. **Accept:** owner CRUD works end to end; non-owner tests remain denied.
- [x] **P1-17 — Import validation and quotas.** Accept only `.md`/`.txt`, enforce UTF-8 and pilot size/count/corpus limits before upstream processing. **Accept:** PDF/DOCX, invalid text, and over-quota input produce clear errors without creating work.
- [x] **P1-18 — Recoverable revisions.** Store note-body revisions with optimistic concurrency and restore support. **Accept:** stale edits return a conflict; prior content can be previewed and restored without losing newer history.

**Gate C — passed 2026-08-26:** A user can safely create, edit, import, recover, and delete owner-only notes; live quota, concurrency, revision, and authorization probes pass. See [the Gate C record](PHASE1_GATE_C.md).

### D. Plain RAG user functions

- [x] **P1-19 — Versioned Plain RAG preset.** Define one approved vector-plus-keyword preset with chunking, rerank, model, and limits; map it through the adapter. **Accept:** the stored profile reproduces the effective upstream configuration.
- [x] **P1-20 — Plain RAG creation and ingestion.** Create Document RAG KBs and expose upload, processing state, failure reason, retry, and cancellation. **Accept:** representative Markdown, PDF, Word, and spreadsheet fixtures reach expected states.
- [x] **P1-21 — Plain RAG retrieval and citations.** Verify hybrid retrieval through normal chat and citation access without graph, pixel, or Neo4j dependencies. **Accept:** English and Chinese sentinel queries return authorized, openable citations.

**Gate D — passed 2026-08-26:** Plain RAG works end to end through WeKnora ingestion and retrieval, with English/Chinese normal-chat citations, and remains independent of future indexing profiles. See [the Gate D record](PHASE1_GATE_D.md).

### E. Optional Note Wiki and Phase 1 release

- [ ] **P1-22 — Wiki estimate.** Estimate eligible note corpus size, model work, time, and quota before enabling a build request. **Accept:** oversized or over-budget requests stop before model execution.
- [ ] **P1-23 — Wiki build lifecycle.** Start, observe, cancel, retry, and incrementally rebuild the derived Wiki without changing source notes. **Accept:** cancellation or failure leaves notes intact and reports an honest state.
- [ ] **P1-24 — UI and exclusion cleanup.** Remove navigation and settings for excluded capabilities while retaining server denial. **Accept:** UI tests and screenshots cover enabled navigation; direct disabled-route probes still fail. The [pre-cleanup browser baseline](PHASE1_UI_EVIDENCE.md) records the remaining visible gaps.
- [ ] **P1-25 — Release regression and packaging.** Run upstream-boundary, gateway, authorization, frontend, Compose, ingestion, retrieval, and upgrade-contract tests; update deployment/offline-image documentation. **Accept:** all Phase 1 gates pass on a clean checkout and the upstream patch ledger remains empty.

## Recommended first implementation

Start only with **P1-01**. Its route-policy matrix defines the gateway contract and prevents later security rework. After review, implement **P1-02** as the first code change.
