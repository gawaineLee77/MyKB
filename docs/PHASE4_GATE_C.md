# Phase 4 Gate C — Hosted MCP Facade

| Field | Result |
|---|---|
| Status | Passed on 2026-08-30 |
| Scope | P4-13 through P4-16 |
| Endpoint | `POST /mcp` |
| Product capability | `mcp: true` |

## Transport and tools

MindCreek exposes a stateless, authenticated Streamable HTTP MCP facade. It implements current `2026-07-28` discovery and tool metadata plus stateless compatibility initialization for the approved 2025 protocol versions. It does not expose hosted stdio, server-sent subscriptions, mutation tools, upstream MCP-service management, or anonymous access.

The initial read-only tools are:

- `list_knowledge_bases`
- `search_knowledge`
- `get_source_excerpt`
- `ask_knowledge_agent`
- `list_publications`
- `list_subscriptions`

Every tool calls product domain services and the Authorized Scope Resolver; none reads the database, vector index, or object storage directly. The transport enforces principal and tenant identity, same-origin browser requests, a 1 MiB request limit, 60 requests per minute per tenant/user, bounded schemas, request correlation, and redacted operation audit.

The accepted live run exercised all six tools, current `2026-07-28` discovery, `2025-11-25` compatibility initialization, non-disclosing scope denial, immediate excerpt revocation, anonymous and wrong-workspace denial, and eight redacted MCP audit events.

## Reproduce

```sh
make phase4-gate-c
```

The live matrix covers discovery, deterministic read-only schemas, all six tools, malformed or anonymous calls, wrong workspace, private KB denial, fresh excerpt authorization, revocation, legacy compatibility, and audit records.
