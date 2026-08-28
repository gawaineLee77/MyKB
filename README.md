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

Phase 0 and Phase 1–3 Gates A–D are complete. The gateway enforces private-by-default RAG KBs, explicit Viewer/Editor grants, live publications and subscriptions, organization-public read access, immediate revocation, and redacted audit records. The product UI adds `My KBs`, `Shared with me`, `Subscribed`, `Discover`, owner-only sharing/publication controls, and permission-aware RAG workspaces. Personal Notes remain owner-only and unpublishable. Product pages are applied by the assertion-checked frontend overlay without modifying the WeKnora submodule:

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
```

The seven-service runtime publishes only the frontend; the gateway and WeKnora app remain private. Phase 3 subscriptions are live references, not content copies. Unfollowed organization-public KBs are readable when explicitly opened but remain outside the default library/agent scope. Corporate OAuth 2.0 and closed registration remain Phase 5.

Docker build definitions, pinned image manifests, and offline-transfer commands live in [`images/`](images/README.md). Current delivery evidence is recorded in the [Phase 3 implementation plan](docs/PHASE3_IMPLEMENTATION_PLAN.md) and [Gate D release record](docs/PHASE3_GATE_D.md). Existing installations should follow the [Phase 0 to Phase 3 upgrade guide](docs/PHASE0_TO_PHASE3_UPGRADE.md). See also the [Phase 2 sharing-model map](docs/PHASE2_SHARING_MODEL.md), [route-action inventory](docs/PHASE2_ROUTE_ACTIONS.md), [build and LAN deployment guide](docs/BUILD_AND_LAN_DEPLOYMENT.md), [overall design](docs/OVERALL_DESIGN.md), and [downstream patch ledger](docs/UPSTREAM_PATCHES.md).
