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

Phase 0 and Phase 1 Gates A–D are complete. The product gateway now enforces owner-only Personal Notes, versioned revisions and quotas, plus guarded Plain RAG creation, multi-format ingestion, hybrid retrieval, normal chat, and citations. Stage 2 product pages are applied by the same assertion-checked frontend overlay without modifying the WeKnora submodule:

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
```

The seven-service Phase 1 runtime publishes only the frontend; the gateway and WeKnora app remain private. Corporate OAuth 2.0, closed registration, sharing/subscriptions, optional Note Wiki, GraphRAG, PixelRAG, and Ontology remain later tasks.

Docker build definitions, pinned image manifests, and offline-transfer commands live in [`images/`](images/README.md). Remaining work follows the [Phase 1 implementation plan](docs/PHASE1_IMPLEMENTATION_PLAN.md). See also the [build and LAN deployment guide](docs/BUILD_AND_LAN_DEPLOYMENT.md), [Gate C](docs/PHASE1_GATE_C.md), [Gate D](docs/PHASE1_GATE_D.md), [overall design](docs/OVERALL_DESIGN.md), and [downstream patch ledger](docs/UPSTREAM_PATCHES.md).
