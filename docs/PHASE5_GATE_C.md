# Phase 5 Gate C Evidence

| Field | Result |
|---|---|
| Engineering status | Local acceptance passed 2026-08-31; Docker Scout result pending backend connectivity |
| Product version | `0.6.0-phase5` |
| Sensitive telemetry | Prohibited and regression-tested |

Local results: ten migrations passed empty/repeat/rollback/forward; 300 concurrent-smoke requests completed with zero errors and p95 `51.9 ms`; isolated restore completed in `26 s`; application failure returned a bounded redacted 502 and recovered in `8 s`.

## Delivered controls

- HTTPS-only production reverse proxy with HSTS, bounded security headers, private dependency network, and explicit application/provider egress.
- Production secret preflight, independent credential policy, permission checks, rotation/recovery procedure, and no secret material in normal data bundles.
- Consistent PostgreSQL, uploaded-file, migration, source-revision, and product-configuration backup with checksums; destructive restore is separately confirmed.
- Non-destructive recovery drill against a temporary database and volume with measured RTO evidence.
- Redacted JSON request events, correlation IDs, private Prometheus metrics, service/capacity/security probe, and documented alert thresholds.
- Bounded load test, ten-migration lifecycle test, controlled application failure injection, inherited authorization suites, and fixable-critical vulnerability policy.

## Acceptance commands

```sh
make phase5-gate-c-static-check
make phase5-migration-probe
make phase5-load-probe
make phase5-observability-probe
make phase5-recovery-drill
make phase5-failure-recovery-probe
MINDCREEK_ALLOW_EXTERNAL_SCANNER=true make phase5-security-scan
```

Reports are written beneath `.local/` with mode `0600`. They retain aggregate results only—never credentials, prompts, answers, document excerpts, or private documents.

The Docker Scout command may disclose runtime-image PURLs and layer digests to Docker's external service and therefore requires the explicit environment acknowledgement shown above. It does not scan the source directory and must not be run implicitly on private images. Disclosure was approved and `docker login` succeeded on 2026-08-31, but the backend request to `hub.docker.com/v2/auth/token` timed out before image evaluation. Rerun after restoring Docker Hub DNS/VPN/proxy connectivity.
