# Phase 0 Baseline and Upstream Inventory

| Field | Verified value |
|---|---|
| Status | Phase 0 exit gates complete |
| Inventory date | 2026-08-25 |
| Product repository | `gawaineLee77/MyKB` (private) |
| Upstream | `Tencent/WeKnora` |
| Approved release | `v0.7.2` |
| Immutable commit | `3d5d8bfcdfeeea266b292b71cea616847af28d0f` |
| Integration form | Git submodule at `upstream/weknora` |
| Downstream patches | 0 |

## Reproducible checkout

```sh
git clone --recurse-submodules https://github.com/gawaineLee77/MyKB.git
cd MyKB
make phase0-check
```

The submodule pointer, release tag, URL, clean worktree, design alignment, and PNG artifact are checked by product-owned scripts. Updates must change the submodule pointer on an `upgrade/weknora-vX.Y.Z` branch and pass compatibility CI. Product code belongs outside `upstream/weknora`.

## Upstream structure

| Area | Upstream location | Phase 0 finding |
|---|---|---|
| Go application | `cmd/server`, `internal/**` | Go 1.26 server, Gin HTTP routing, `dig` dependency injection, GORM repositories, Asynq jobs. |
| Web application | `frontend/**` | Vue 3, TypeScript, Vite; upstream CI uses Node 24. |
| Document reader | `docreader/**` | Python 3.10.18+ parser service with PDF, Word, Markdown, spreadsheet, EPUB, HTML/MHTML, image, and optional MarkItDown routes. |
| MCP server | `mcp-server/**` | Python package v1.1.1; stdio, legacy SSE, and Streamable HTTP transports. |
| Data and jobs | `migrations/**`, `internal/router/task.go` | PostgreSQL migrations through 000079; Redis/Asynq parsing and dedicated Wiki queues. |
| Deployment | `docker-compose.yml`, `docker-compose.dev.yml`, `helm/**` | Standard Compose, local dependency Compose, Helm, and Lite build paths. |
| Excluded clients | `miniprogram/**`, `cli/**`, `internal/im/**` | Keep in the submodule; do not build, route, start, or expose in the MyKB distribution. |

## Relevant upstream capabilities

### Personal Notes foundation

- `POST /api/v1/knowledge-bases/:id/knowledge/manual` and `PUT /api/v1/knowledge/manual/:id` provide sanitized Markdown creation and editing.
- Manual entries support `draft` and `publish`; metadata stores content, Markdown format, update time, and a monotonically incremented version. The live v0.7.2 API rejects the `published` value shown in its API-reference example.
- The backend accepts at most 200,000 Unicode characters per manual entry. MyKB will apply its smaller Note Space limits at the gateway.
- The frontend already contains a manual Markdown editor, preview, draft state, and document cards.
- Wiki pages have persistent revision history, but manual-note versions do not preserve historical bodies. Note revision recovery therefore remains a product companion capability.

### Plain RAG foundation

- `DefaultIndexingStrategy()` enables vector and keyword indexing while leaving Wiki and graph disabled.
- `HybridSearch` authorizes every requested KB, checks embedding-space compatibility, fans out by vector store, fuses vector/keyword results, and preserves citations.
- Upstream supports configurable chunking, parent/child chunks, reranking, multiple vector stores, parser-engine selection, processing status, retry, and cancellation.
- This is sufficient for the Plain RAG MVP through a versioned adapter and approved presets; no retriever fork is planned.

### Wiki, graph, and visual foundations

- Wiki generation is independently enabled by `indexing_strategy.wiki_enabled`; page CRUD, graph view, linting, source references, revisions, and revert already exist.
- A single document contributes at most 32,768 characters to Wiki LLM ingestion. Upstream does not expose MyKB's required preflight cost/quota approval.
- Graph extraction and Neo4j storage exist. The chat pipeline can expand entity matches through Neo4j, while the `query_knowledge_graph` agent tool currently delegates its main query to `HybridSearch` and states that full graph query-language support is still under development. Treat this as a benchmark candidate, not completed GraphRAG.
- VLM/image-processing configuration exists, but PixelRAG page/tile embeddings, region citations, and lifecycle isolation do not. These remain a future sidecar.

## Authorization and sharing findings

WeKnora v0.7.2 has creator-aware mutation guards, per-route KB access middleware, API-key capabilities and allow-lists, organization sharing, shared agents, and negative authorization tests. However, `ListKnowledgeBases` intentionally returns all KBs owned by the current tenant/workspace, and same-tenant KBs pass service-level hybrid-search authorization. That is broader than MyKB's user-private Personal Notes policy.

Consequences:

1. WeKnora stays on a private network and is never called directly by browsers or external agents.
2. The Product Gateway resolves user-level policy and allowed KB IDs before calling upstream.
3. Companion product data distinguishes owner-private, explicit grants, publication, and subscription without changing upstream tables.
4. Phase 2 must prove that list, detail, preview, download, Wiki, chat, and every retrieval path are denied for an unauthorized same-workspace user.

## MCP finding

The upstream network MCP transport requires one `MCP_SERVER_AUTH_TOKEN` and calls WeKnora with one configured `WEKNORA_API_KEY`. It is useful for protocol and tool reuse but does not by itself represent each MyKB end user or apply publication/subscription policy. The production MCP endpoint must therefore be a product facade that authenticates an individual/delegated principal, resolves MyKB scope, and invokes only approved upstream API capabilities. Hosted stdio remains disabled.

## Router and extension seams

`internal/router/router.go` is the composition root and registers all route domains under `/api/v1`. Existing seams worth preserving are interfaces under `internal/types/interfaces`, `dig` providers in `internal/container`, REST contracts, scoped API keys, processing jobs, and computed KB capabilities.

The product will initially use:

- Reverse-proxy denial for excluded public routes.
- A Product Gateway/BFF for modes, permissions, catalog, subscriptions, MCP, and allowed-KB resolution.
- Product-owned tables keyed by opaque upstream IDs.
- REST adapters for manual knowledge, KB lifecycle, hybrid search, Wiki, jobs, and citations.
- No imports of upstream `internal/**` packages and no product migrations in the upstream migration directory.

## Deployment inventory

The standard Compose file includes frontend, app, parser, sandbox, PostgreSQL, Redis, optional object/vector/graph services, optional observability, and MCP. It also contains services that MyKB will not start by default. The initial product profile should include only the gateway/frontend, WeKnora app, PostgreSQL, Redis, object storage, one selected vector backend, DocReader, and approved model endpoints. Neo4j, MCP, OIDC, and observability are explicit profiles; IM, embed, Web search, cloud, and unused vector backends are disabled.

## Local environment result

| Tool | Local result | Required action |
|---|---|---|
| Git | Available | Baseline and submodule verified. |
| Go | Go 1.26.7 | Backend vet, tests, and server build passed. Full Xcode is selected command-locally for native C++ headers. |
| Node/npm | Node 25.8.1 / npm 11.11.0 | Use Node 24 for parity with upstream CI. |
| Python | 3.14.3 | Use isolated Python 3.12 for MCP and 3.10.18+ for DocReader. |
| Docker/Compose | 29.3.0 / v5.1.0 | Isolated v0.7.2 runtime and synthetic probes passed. |

## Validation result

| Check | Result |
|---|---|
| Immutable submodule boundary | Passed: exact v0.7.2 tag and commit, clean upstream worktree, zero patches. |
| Bilingual design structure | Passed: 116 aligned headings, balanced code fences, valid v0.4 PNG. |
| MCP server | Passed: 23/23 upstream `unittest` cases on isolated CPython 3.12.13. |
| Frontend tests | Passed: 341/341 upstream tests on the local Node 25 runtime. |
| Frontend type-check | Passed: `vue-tsc --build`. |
| Frontend production build | Passed: Vite 7.3.6; emitted only upstream bundle-size warnings. |
| Go backend | Passed: CI-equivalent `go vet`, package tests, and server build on Go 1.26.7. |
| Compose smoke test | Passed: exact v0.7.2 image revision, six-service isolated profile, healthy backend/dependencies, frontend HTTP 200. |
| Synthetic Plain RAG | Passed: English and Chinese Markdown sentinels ingested and retrieved. |
| Cross-workspace probe | Passed: Bob received HTTP 403 for Alice's KB before joining Alice's workspace. |
| Same-workspace Personal Notes probe | Known gap confirmed: Viewer Bob could list, open, and retrieve Alice's private synthetic note. |

The frontend will be rechecked under the upstream-supported Node 24 runtime in GitHub Actions; the successful Node 25 result is useful but is not the compatibility authority.

## Phase 0 exit checklist

- [x] Connect the private product repository.
- [x] Pin an unmodified, identifiable WeKnora release.
- [x] Record source structure, services, routes, migrations, UI, and extension seams.
- [x] Confirm Personal Notes and Plain RAG foundations from source.
- [x] Identify privacy, MCP-principal, Wiki-cost, GraphRAG, and PixelRAG gaps.
- [x] Add deterministic boundary and document checks.
- [x] Add upstream compatibility CI configuration.
- [x] Run upstream frontend tests, type-check, and production build locally.
- [x] Run upstream MCP tests in isolated Python 3.12.
- [x] Run the upstream CI-equivalent backend suite locally with Go 1.26.
- [x] Run an isolated Compose smoke test with a deterministic non-secret model endpoint.
- [x] Create synthetic users and a non-sensitive multilingual evaluation corpus.
- [x] Execute black-box cross-workspace and same-workspace privacy/retrieval probes.

Phase 0 is complete. See [Phase 0 runtime report](PHASE0_RUNTIME_REPORT.md) for reproducible runtime evidence and the confirmed authorization gap that Phase 1 must address.
