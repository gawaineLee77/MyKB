# Phase 2 Gate D Acceptance

| Field | Value |
|---|---|
| Status | Passed on 2026-08-27 |
| Scope | P2-18 regression and upgrade contract |
| Product release | MindCreek `0.3.0-phase2` |
| Upstream | WeKnora v0.7.2, unmodified |

## Release contract

Phase 2 uses `config/phase2-capabilities.json` as its fail-fast release registry and publishes `mindcreek-gateway:phase2`. The offline image manifest includes the gateway alongside the six runtime dependencies, and retains the earlier `phase1` gateway tag only as a local compatibility alias.

The candidate-upstream check validates required WeKnora route and wire-type seams, then injects the MindCreek route inventories into an untouched candidate tree and runs both coverage contracts. A future candidate can be checked with `MINDCREEK_CANDIDATE_WEKNORA=/path/to/weknora make phase2-upstream-contract-check`.

The clean-copy check reconstructs the complete working change on a fresh local clone, initializes the pinned submodule, proves the checkout is clean, compiles every gateway package, verifies overlays and static acceptance records, and validates the deployment topology. It does not require a running service or private fixture.

## Regression evidence

- Phase 1 Gates A–D remain the inherited identity, isolation, Personal Notes, and Plain RAG baseline.
- Phase 2 policy, migration, sharing, revocation, audit, and negative live matrices passed through the public frontend endpoint.
- The production frontend image ran 345 tests, completed TypeScript checking, and produced the Vite bundle.
- The pinned upstream and candidate adapter contracts passed; `docs/UPSTREAM_PATCHES.md` remains empty.
- The reconstructed clean checkout passed source, gateway compile, overlay, upstream-boundary, and Compose configuration checks.

Per user direction, screenshots are deferred to the GPT app and are not a Phase 2 completion blocker.

## Reproduce

```sh
make phase2-check
make phase2-gate-b
make phase2-gate-c
make phase2-gate-d
```

`make phase2-gate-d` runs the complete static/unit regression and the clean-copy release check. Live Gate B and the production UI build stay explicit because they require the local container runtime.
