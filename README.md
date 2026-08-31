# MindCreek

MindCreek (repository: MyKB) is an upstream-first internal knowledge-base platform built around Tencent WeKnora. The product targets private Personal Notes, multi-format Plain RAG, governed sharing and subscriptions, an authorized agent/MCP surface, and later GraphRAG, PixelRAG, and ontology-guided knowledge graphs.

## Bootstrap

```sh
git clone --recurse-submodules https://github.com/gawaineLee77/MyKB.git
cd MyKB
make phase0-check
make upstream-test-go
make phase0-compose-config
make phase0-up
make phase0-runtime-check
make phase0-probe
```

WeKnora is pinned as an unmodified submodule under `upstream/weknora`. Product code must be added outside that boundary. Do not develop on a branch inside the submodule or commit unrecorded upstream changes.

## Current status

Phase 0 and Phase 1–5 engineering Gates A–D are complete. Phase 5 adds zero-key managed chat, embedding, and rerank defaults, corporate OIDC and closed registration, a TLS-only production profile, recovery/observability controls, and controlled-pilot evidence. The product UI provides `My KBs`, `Shared with me`, `Subscribed`, `Discover`, `Authorized Ask`, and managed model status; Personal Notes remain owner-only and unpublishable. Product pages are applied by the assertion-checked frontend overlay without modifying the WeKnora submodule:

```sh
make stage1-check
make stage1-compose-config
make stage1-ui-build
make stage1-up
make stage1-runtime-check
make phase1-gateway-build-offline
make phase1-up
make phase1-gate-c
make phase1-gate-d
make phase2-check
make phase2-gate-a
make phase2-gate-b
make phase2-gate-c
make phase2-gate-d
make phase3-check
make phase3-gate-a
make phase3-gate-b
make phase3-gate-c
make phase3-gate-d
make phase4-check
make phase4-gate-a
make phase4-gate-b
make phase4-gate-c
make phase4-gate-d
make phase5-compose-config
make phase5-build-offline
make phase5-up
make phase5-gate-a
make phase5-gate-b
make phase5-gate-c
make phase5-gate-d
```

The seven-service runtime publishes only the frontend; the gateway and WeKnora app remain private. Subscriptions are live references, not content copies. Unfollowed organization-public KBs are readable when explicitly selected but remain outside the default library/agent scope. Phase 5 provides managed defaults, corporate OAuth/OIDC, first-login provisioning, closed registration, suspension, break-glass controls, production hardening, and reproducible pilot tooling. See the [Phase 5 implementation plan](docs/PHASE5_IMPLEMENTATION_PLAN.md), [operations guide](docs/PHASE5_OPERATIONS.md), and [Gate D evidence](docs/PHASE5_GATE_D.md).

Docker build definitions, pinned image manifests, and offline-transfer commands live in [`images/`](images/README.md). See also the [identity guide](docs/PHASE5_IDENTITY_PROVIDER.md), [backup/recovery guide](docs/PHASE5_BACKUP_RECOVERY.md), [pilot guide](docs/PHASE5_PILOT.md), [build and LAN deployment guide](docs/BUILD_AND_LAN_DEPLOYMENT.md), [overall design](docs/OVERALL_DESIGN.md), and [downstream patch ledger](docs/UPSTREAM_PATCHES.md).
