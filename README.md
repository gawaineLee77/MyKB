# MyKB

MyKB is an upstream-first internal knowledge-base platform built around Tencent WeKnora. The product targets private Personal Notes, multi-format Plain RAG, governed sharing and subscriptions, an authorized agent/MCP surface, and later GraphRAG, PixelRAG, and ontology-guided knowledge graphs.

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

Phase 0 is complete. The pinned v0.7.2 source and images were verified at the same commit; backend, frontend, MCP, Compose, bilingual Plain RAG, and two-user authorization probes passed. The probes confirmed that upstream same-workspace access is too broad for owner-private Personal Notes, so Phase 1 begins with the Product Gateway authorization boundary.

See [Phase 0 baseline](docs/PHASE0_BASELINE.md), [runtime report](docs/PHASE0_RUNTIME_REPORT.md), [overall design](docs/OVERALL_DESIGN.md), and [downstream patch ledger](docs/UPSTREAM_PATCHES.md).
