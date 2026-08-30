# Phase 4 Implementation Plan

| Field | Value |
|---|---|
| Status | Complete |
| Started | 2026-08-28 |
| Completed | 2026-08-30 |
| Upstream baseline | Unmodified WeKnora v0.7.2 |
| Outcome | Web and MCP agents safely query the caller's authorized MindCreek knowledge scope |

## Boundaries and invariants

Phase 4 adds one product-owned Authorized Scope Resolver. Web search, quick RAG, ReAct tools, mentions, session replay, citations, and MCP tools must use the same resolver before retrieval. Default agent scope contains owned, explicitly shared, and actively subscribed KBs. An organization-public KB is readable but enters agent scope only when explicitly selected or actively subscribed. Personal Notes remain owner-only. Scope loss must take effect on the next request.

WeKnora remains unmodified and continues to own parsing, Plain RAG indexing, retrieval, sessions, and answer generation. MindCreek rewrites only authenticated retrieval scope at the gateway boundary. Hosted MCP uses Streamable HTTP, starts read-only, accepts no anonymous or hosted-stdio access, and never queries storage or indexes directly.

## Task list

### A. Authorized-scope foundation

- [x] **P4-01 — Scope contract.** Define default and explicit selection, deterministic entries, limits, typed errors, and non-disclosing denial.
- [x] **P4-02 — Authorized Scope Resolver.** Resolve owned, user-granted, subscribed, and explicitly selected organization-public KBs through Phase 3 authorization decisions.
- [x] **P4-03 — Session scope lifecycle.** Persist effective session KB scope and reauthorize it on every replay, message, continuation, and citation request.
- [x] **P4-04 — Retrieval request normalization.** Replace caller and upstream-agent scope with the exact resolved KB list; disable excluded web-search and upstream-MCP selections.
- [x] **P4-05 — Agent/tool audit migration.** Store redacted web/MCP operation metadata, scope IDs, timing, outcome, and correlation without prompts or excerpts.
- [x] **P4-06 — Scope negative matrix.** Cover owner, viewer, editor, subscriber, public-unselected, public-selected, revoked, unpublished, wrong workspace, and Personal Notes.

**Gate A:** The resolver and audit foundation pass unit and migration lifecycle tests before retrieval behavior is enabled.

### B. Unified web agent

- [x] **P4-07 — Search and chat enforcement.** Apply resolved IDs to knowledge search, quick RAG, ReAct, custom agents, Wiki tools, and KB/file mentions.
- [x] **P4-08 — Grounded references.** Require returned and subsequently opened sources to remain inside the effective scope with fresh authorization.
- [x] **P4-09 — Scope APIs.** Expose selectable default KBs and explicit-scope previews without leaking inaccessible metadata.
- [x] **P4-10 — Ask workspace.** Add product-owned KB selection, quick/agent mode, streaming answer state, and source display.
- [x] **P4-11 — Live web matrix.** Verify scope intersection, revocation, publication loss, session continuity, and source access end to end.
- [x] **P4-12 — Retrieval baseline.** Measure multilingual grounded retrieval quality and latency on synthetic representative content.

**Gate B:** Web agent retrieval is safe and grounded before MCP is enabled.

### C. Hosted MCP facade

- [x] **P4-13 — Streamable HTTP transport.** Implement authenticated JSON-RPC initialize, ping, tool discovery, and tool calls at `/mcp`.
- [x] **P4-14 — Read-only tools.** Add `list_knowledge_bases`, `search_knowledge`, `get_source_excerpt`, `ask_knowledge_agent`, `list_publications`, and `list_subscriptions` through domain services.
- [x] **P4-15 — MCP controls.** Enforce principal identity, workspace, scope limits, rate limits, payload limits, request IDs, and redacted audit.
- [x] **P4-16 — Protocol/security matrix.** Test malformed JSON-RPC, anonymous access, invalid tools/arguments, cross-scope IDs, revocation, and result schemas.

**Gate C:** Authenticated agents can use the read-only MCP facade without creating a parallel authorization path.

### D. Release and compatibility

- [x] **P4-17 — Regression and candidate contract.** Run inherited gates, upstream suites, Phase 4 tests, production UI build, live probes, and candidate-upstream checks.
- [x] **P4-18 — Phase 4 release.** Publish capability registry, image/version metadata, operations documentation, clean-copy evidence, and Gate A-D records.

**Gate D:** MindCreek Phase 4 is reproducible from a clean checkout and the upstream patch ledger remains empty.
