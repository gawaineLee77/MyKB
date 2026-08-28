# Phase 3 Gate D — Release and Compatibility

| Field | Value |
|---|---|
| Status | Passed on 2026-08-27 |
| Scope | P3-18 release contract |
| Product release | MindCreek `0.4.0-phase3` |
| Upstream | WeKnora v0.7.2, unmodified |

## Release contract

Phase 3 selects `config/phase3-capabilities.json` and publishes `mindcreek-gateway:phase3`. The offline builder also creates `phase2` and `phase1` compatibility aliases for existing ignored local environments. Deployment keeps the WeKnora app private and exposes product traffic only through the MindCreek frontend and gateway.

The candidate-upstream contract validates the unchanged WeKnora routes and wire fields used by the product adapter, then runs the Phase 1 and Phase 2 route inventories against the candidate source. Phase 3 introduces no upstream patch or new direct dependency on WeKnora `internal/**` packages.

The clean-copy check reconstructs all tracked and untracked work in a fresh clone, initializes the pinned submodule, checks the immutable boundary and every Phase 3 static acceptance record, compiles the gateway, and validates Compose configuration without relying on a running stack.

## Reproduce

```sh
make phase3-check
make phase3-gate-b
make phase3-gate-c
make phase3-gate-d
```

Live and frontend image checks remain explicit because they require Docker. Synthetic reports stay under `.local/` and never contain private documents or credentials.
