# Phase 2 Gate B Acceptance

| Field | Value |
|---|---|
| Status | Passed on 2026-08-27 |
| Scope | P2-07 through P2-14 |
| Upstream | WeKnora v0.7.2, unmodified |
| Entry point | `http://127.0.0.1:18080` |

## Delivered controls

The gateway loads the pinned Phase 2 route-action inventory at startup and authorizes every resolved knowledge-base reference through one decision service. Direct KB paths and indirect document, chunk, FAQ, Wiki, search, chat, citation, session, agent, preview, and download references fail closed. Upstream list responses are filtered before they reach clients.

Product endpoints provide paginated `owned` and `shared` views, tenant-user lookup, and owner-only grant list/create/update/revoke operations. Grants support Viewer or Editor permission, optional expiry, optimistic revisions, and immediate revocation. Personal Notes reject all sharing requests.

Viewer permits discovery, read, retrieval, chat, and citations. Editor additionally permits content ingestion and the limited `name`/`description` metadata update. Neither role can manage grants, change unsafe KB configuration, transfer ownership, publish, or delete the KB.

Session-to-KB scope is stored as a union and reauthorized on later requests. Audit records cover grant lifecycle and denied high-value actions with actor, target, outcome, correlation ID, and redacted structured old/new values.

## Verification

```sh
make phase2-check
make phase1-gateway-build-offline
make phase2-gate-b
```

The synthetic live matrix proved:

- same-workspace administrator and wrong-workspace users receive non-disclosing responses before a grant;
- Viewer read/search/chat/citation succeeds while all mutations fail;
- Editor content and limited metadata changes succeed while grant, unsafe configuration, and delete operations fail;
- revoked and expired users disappear from `Shared with me` and cannot reuse direct, citation, chunk, or session URLs;
- stale revisions return typed conflicts;
- Personal Notes and unsupported group subjects cannot be granted;
- correlated audit records contain no document, prompt, or content payload.

The generated local evidence is `.local/phase2-gate-b-report.json`; it is synthetic and intentionally excluded from version control.

## Residual scope

Organization-public access and subscriptions remain Phase 3. MCP scope remains Phase 4. Corporate OAuth 2.0 and closed registration remain Phase 5.
