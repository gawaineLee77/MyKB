# Phase 4 Gate A — Authorized Scope Foundation

| Field | Result |
|---|---|
| Status | Passed on 2026-08-30 |
| Scope | P4-01 through P4-06 |
| Upstream | Unmodified WeKnora v0.7.2 |
| Product migrations | Nine total; migration 9 is Phase 4 |

## Foundation

`agentscope.Resolver` is the sole product scope contract for Web and MCP agent operations. Default scope is the deterministic union of owned, explicitly shared, and actively subscribed KBs. Organization-public KBs require explicit selection unless subscribed. Explicit requests are bounded to 64 KBs, authorize every ID for read, and collapse inaccessible, revoked, unpublished, and wrong-workspace resources into one non-disclosing denial.

The gateway rewrites retrieval bodies to the exact resolved IDs, removes singular alternatives, and forces excluded web search and upstream MCP selections off. Effective session KBs are persisted and reauthorized on later message, replay, and source requests. Personal Notes continue through owner-only authorization.

Migration `000009_phase4_agent_operations` adds an isolated audit table containing identity, client kind, operation, KB IDs, outcome, error code, correlation ID, duration, and timestamp. It has no prompt, answer, excerpt, document, or chunk-content column. Successful retrieval fails closed if its audit event cannot be stored.

## Reproduce

```sh
make phase4-gate-a
```

This runs static boundaries, focused scope/audit/access tests, and the nine-migration empty/repeat/down/up lifecycle.
