# WeKnora v0.8.0 Upgrade Record

> Current upstream upgrade reference. Repository validation is distinct from target-server rollout; follow the [deployment runbook](../guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md) and retain explicit production approval.

## Decision

MindCreek promotes the unmodified WeKnora `v0.8.0` release at commit `1edcd54b43606d9079bb36650efe3f68707a79ea`, replacing `v0.7.2` at `3d5d8bfcdfeeea266b292b71cea616847af28d0f`. The upstream submodule remains the only source of WeKnora code; the downstream patch ledger remains empty.

## Product compatibility

- The complete upstream route inventory now contains 429 routes; 170 are knowledge-base-policy-controlled.
- Session artifacts and message-scoped files are authorized as KB reads.
- New skill catalog, sandbox configuration, personal environment-variable, long-term memory, sandbox-check, and local admin-user creation routes are denied at the Product Gateway. Their settings and agent-editor entry points are also hidden. These v0.8.0 features are outside the current MindCreek scope.
- The MindCreek frontend overlay supports both the existing upstream `patch()` helper and the new agent-editor and mention-import layout.
- Runtime manifests, compose defaults, probes, the version adapter, and operator documentation now pin `v0.8.0`.

## Database and rollback

WeKnora automatically applies upstream PostgreSQL migrations `000080`–`000090` on first v0.8.0 startup. They add auto-tag settings, chat artifacts and usage, sandbox and skill metadata, environment-variable storage, and long-term-memory tables. MindCreek does not expose the out-of-scope route families, but their additive schema objects are still created by the stock app.

Before deployment, run `make phase5-backup` and verify the bundle with `make phase5-recovery-drill`. Never start v0.7.2 and v0.8.0 against the same volumes. To roll back only MindCreek product services while retaining the v0.8.0 app, stop the stack and restore the previous product images. To roll the upstream app back to v0.7.2, restore the pre-upgrade database backup unless reuse of the v0.8.0 schema has been rehearsed separately.

## Validation evidence

The promotion gate completed on 2026-09-10:

```sh
make phase0-check
make phase1-route-policy-check
make phase2-sharing-model-check
make phase2-route-actions-check
make phase1-gateway-test
make phase5-check
make upstream-test-go
make upstream-test-frontend
make upstream-test-mcp
```

The Go suite passed vet, unit tests, and server build. The frontend passed tests, type-check, and production build. All 23 MCP tests passed. The Phase 5 contract and overlay checks passed against the candidate before the submodule pin was promoted.

An isolated PostgreSQL rehearsal created a complete v0.7.2 schema, inserted a synthetic sentinel row, and then ran the stock v0.8.0 migrations. It finished at `schema_migrations=90:false`; the sentinel survived and representative migration-90 tables were present. The temporary Docker network and database container were removed after the test.

## Deployment sequence

```sh
git submodule update --init --recursive
make upstream-status
make phase5-backup
make phase5-images-pull-amd64
make phase5-images-build-amd64
make phase5-images-save-amd64
make phase5-production-compose-config
./scripts/phase5-production-compose.sh up -d
make phase5-runtime-check
make phase5-migration-probe
```

Keep the previous image archive and verified backup until the observation window closes. Production promotion still requires live identity, model, ingestion, retrieval, sharing, subscription, revocation, and MCP smoke tests.
