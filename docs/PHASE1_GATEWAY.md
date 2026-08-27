# Phase 1 Gateway

MindCreek Phase 1 runs a product-owned Go gateway in front of the pinned, unmodified WeKnora v0.7.2 service. Nginx sends `/api/**`, `/files`, and `/r/**` to `gateway:8080`; only the frontend is published to the host. The gateway denies excluded routes before they reach WeKnora and rejects routes absent from the verified inventory.

## Product endpoints

| Endpoint | Purpose |
|---|---|
| `GET /health` | Gateway container liveness |
| `GET /version` | MindCreek version and supported WeKnora version |
| `GET /api/v1/capabilities/knowledge-modes` | Authoritative Phase 1 capability document |
| `POST /api/v1/knowledge-spaces` | Idempotent Personal Notes or Plain RAG creation |
| `GET /api/v1/knowledge-bases/{id}/product-profile` | Stored product mode and effective preset |
| `/api/v1/knowledge-bases/{id}/notes/**` | Owner-only note CRUD, import, revisions, and restore |
| `/api/v1/knowledge-bases/{id}/ingestions/**` | Guarded Plain RAG upload, status, retry, and cancel |

Personal Notes and Plain RAG are enabled through product-owned creation and workspace pages. IM, Mini Program, CLI, embed, Web search, external connectors, hosted MCP, GraphRAG, PixelRAG, and Ontology remain disabled.

## Configuration

The deployment mounts these reviewed files read-only:

- `config/phase1-route-policy.json` — ordered route classifications tied to WeKnora v0.7.2 and commit `3d5d8bf`.
- `config/phase1-capabilities.json` — the complete advertised capability set.

The gateway refuses unsupported upstream versions, malformed registries, missing flags, and premature feature enablement. Connection settings use `MINDCREEK_UPSTREAM_URL`, `MINDCREEK_UPSTREAM_VERSION`, and `MINDCREEK_UPSTREAM_TIMEOUT`.

## Developer workflow

```sh
make phase1-check
make phase1-compose-config
make phase1-build
make phase1-up
make phase1-runtime-check
make phase1-gate-c
make phase1-gate-d
make phase1-down
```

If Docker Hub is temporarily unavailable but Go 1.26 is installed, `make phase1-gateway-build-offline` cross-compiles a static Linux binary and packages it in a base-free image. The normal multi-stage image remains the release build path.

`phase1-runtime-check` confirms seven healthy/running services, no published app or gateway bypass port, frontend-to-gateway routing, the MindCreek UI, all disabled-route probes, and a clean upstream submodule. It uses synthetic requests only and preserves volumes on shutdown.

KB-policy-controlled routes resolve the authenticated upstream principal and apply product profiles before proxying. Personal Notes uses exact-owner read/write policy, list filtering, indirect resource resolution, retrieval-scope checks, and fail-closed derived-content handling. Plain RAG validates its stored versioned preset, then delegates file parsing, chunking, indexing, hybrid retrieval, chat, and citations to WeKnora. See the [Gate B](PHASE1_GATE_B.md), [Gate C](PHASE1_GATE_C.md), and [Gate D](PHASE1_GATE_D.md) records.
