# Phase 4 Gate B — Unified Web Agent

| Field | Result |
|---|---|
| Status | Passed on 2026-08-30 |
| Scope | P4-07 through P4-12 |
| Entry point | `http://127.0.0.1:18080` |
| Upstream | WeKnora v0.7.2, unmodified |

## Delivered behavior

The product gateway applies the same authorized scope to cross-KB search, quick RAG, ReAct, fixed custom agents, KB/file mentions, and continued sessions. It rejects fixed-agent scope expansion, validates non-streaming search results against the resolved IDs, and requires fresh authorization whenever a source chunk is opened. WeKnora still performs retrieval and answer generation.

The product-owned Ask workspace exposes default and explicit KB selection, Quick Answer and Smart Reasoning modes, streaming state, and source links. Owned, shared, and subscribed knowledge is initially selected. Organization-public catalog items are displayed separately and never selected implicitly.

The synthetic live probe creates multilingual shared, private, and organization-public KBs. It measures Recall@5 and request latency for English and Chinese queries, checks grounded quick and reasoning answers, then verifies grant revocation, session replay denial, citation denial, and unpublish behavior. Reports are written under `.local/` and use generated content only.

The accepted run achieved Recall@5 `1.0` for both synthetic English and Chinese queries, with a maximum retrieval latency of `39.62 ms`. Quick Answer and Smart Reasoning were grounded, authorization loss took effect immediately, and four redacted agent audit events were verified.

## Reproduce

```sh
make phase4-up
make phase4-gate-b
```

UI screenshots are a manual release artifact; they do not replace authorization tests.
