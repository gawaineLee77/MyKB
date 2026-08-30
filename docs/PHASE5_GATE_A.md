# Phase 5 Gate A — Managed Models

| Field | Result |
|---|---|
| Status | Passed on 2026-08-30 |
| Product version | `0.6.0-phase5` |
| Upstream | Unmodified WeKnora v0.7.2 |
| Acceptance profile | Synthetic development providers; production secrets not installed |

## Delivered contract

MindCreek declares three organization-managed built-ins with stable IDs: `builtin-mindcreek-chat`, `builtin-mindcreek-embedding`, and `builtin-mindcreek-rerank`. WeKnora remains the model runtime and encrypted credential store. A product-owned template contains environment references, never values, and is mounted read-only. The renderer requires all production inputs, validates provider identifiers and URLs, rejects test endpoints/placeholders outside development, and writes mode `0600`.

`GET /api/v1/mindcreek/models` is the browser-facing model contract. It returns only ID, display name, type, managed/default flags, scope, and availability with `Cache-Control: no-store`. Normal creation injects managed embedding and chat IDs server-side and records the managed reranker in the immutable product profile. Smart Reasoning uses the managed chat/rerank pair with web search disabled.

When the Phase 5 model service is active, raw browser-side WeKnora model create/update/delete, credential, debug, connection-test, and Ollama mutation routes fail closed. The private gateway adapter still uses those upstream seams for governed operations, so users cannot bypass the capability, role, allow-list, quota, or audit controls.

## Advanced overrides

`user_model_overrides` is false by default. The explicit opt-in registry requires matching runtime configuration, a 32-byte `SYSTEM_AES_KEY`, exact host and provider allow-lists, and an external-provider disclosure. Only workspace Owner/Admin roles may create, test, replace, or delete overrides. Credentials are write-only, encrypted by WeKnora, cleared from the browser form after each request, quota-limited to 12, and never returned by the facade. Managed built-ins cannot be mutated through these routes.

## Security and compatibility evidence

| Risk | Evidence |
|---|---|
| Response/log disclosure | Narrow DTO tests, browser API contract check, live response scan, live application-log scan |
| SSRF/malformed provider | Exact host/provider, HTTPS, userinfo/query/fragment, type, key, and dimension tests |
| Cross-user mutation | Owner/Admin gate plus upstream active-workspace model isolation; Viewer denial test |
| Unhealthy defaults | Exact ID/type/builtin/default/active readiness test; creation fails closed with typed errors |
| Credential rotation | Separate credential-resource update test; metadata update never carries a stored key |
| Existing KB compatibility | Stable IDs and persisted effective profile; no operation rewrites an existing KB embedding ID |
| Restart/rollback | Fresh seven-service boot passed; default-off capability and read-only declarations provide rollback without schema changes |

## Verification

```sh
make phase5-compose-config
make phase5-build-offline
make phase5-up
make phase5-runtime-check
make phase5-gate-a
```

The live synthetic probe created a new user, created a Plain RAG space without model fields, ingested Markdown, retrieved two grounded hits, exercised normal Ask with an openable citation, and completed Smart Reasoning. No user key was entered. Real production provider activation remains an operator action using `.local/mindcreek.env`; secrets must not be supplied through chat or committed.
