# Phase 3 Gate B — APIs and Enforcement

| Field | Value |
|---|---|
| Status | Passed on 2026-08-27 |
| Scope | P3-07 through P3-14 |
| Entry point | `http://127.0.0.1:18080` |
| Upstream | WeKnora v0.7.2, unmodified |

## Delivered behavior

Owner-only publication endpoints support create, update, inspect, and unpublish with typed, non-disclosing failures. The audience-filtered catalog supports text, tag, owner, access-mode, updated-after, and pagination filters. Subscription endpoints support list, subscribe, unsubscribe, and mark-seen with retry-safe responses.

Every existing Phase 2 read route continues through the central authorization decision. Subscriber access and organization-public access are read-only; original-source download remains denied. Explicit Viewer/Editor grants remain independent. Audience loss or unpublish immediately removes publication-derived access and inactivates affected subscriptions. Deletion unpublishes before the upstream asynchronous deletion request is forwarded.

The authorized library adds `subscribed` and `all`. `all` is the union of owned, explicitly shared, and actively subscribed KBs; merely discoverable organization-public KBs are not added to default scope. This preserves Phase 4's separation between readable and automatically selected agent knowledge.

## Live acceptance

```sh
make phase1-gateway-build-offline
make phase1-up
make phase3-gate-b
```

The synthetic matrix covers private denial, Personal Notes denial, catalog audience, idempotent subscription, read/search, download denial, update badges, unsubscribe, organization-public direct read, audience loss, unpublish, and redacted audit records. It writes `.local/phase3-gate-b-report.json`.
