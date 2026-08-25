# Phase 0 Runtime Report

| Field | Result |
|---|---|
| Execution date | 2026-08-25 |
| WeKnora image | `wechatopenai/weknora-app:v0.7.2` |
| Image revision label | `3d5d8bfcdfeeea266b292b71cea616847af28d0f` |
| Local toolchain | Go 1.26.7, Docker 29.3.0, Compose 5.1.0 |
| Runtime status | Passed |

## Backend compatibility

The product-owned Go wrapper ran the same package selection as upstream CI: `go vet`, `go test`, and `go build ./cmd/server`, excluding only the separately validated DocReader bridge packages. All gates passed. On macOS, the wrapper selects the installed full Xcode toolchain so the native `gojieba` dependency can locate C++ standard headers. Upstream source remained clean.

## Isolated runtime

`make phase0-up` started six services: frontend, app, DocReader, PostgreSQL, Redis, and a deterministic test-only embedding sidecar. App, DocReader, PostgreSQL, and the sidecar reported healthy; the frontend and backend returned HTTP 200. Only the frontend (`127.0.0.1:18080`) and backend (`127.0.0.1:18081`) were published to the host.

MCP, Neo4j, MinIO, IM sidecars, sandbox, Web search, and alternate vector stores were not started. The app reported zero configured IM channels.

## Synthetic black-box probe

The probe created invented Alice and Bob users and indexed English and Chinese Markdown fixtures.

| Probe | Observed result |
|---|---|
| Bob accesses Alice's KB from Bob's workspace | Denied, HTTP 403 |
| Bob joins Alice's workspace as Viewer and lists Alice's KB | Allowed |
| Bob opens Alice's owner-private KB | Allowed, HTTP 200 |
| Bob retrieves Alice's private sentinel through hybrid search | Allowed, one result |
| English Plain RAG sentinel | Retrieved |
| Chinese Plain RAG sentinel | Retrieved |

This confirms the Product Gateway must enforce Personal Notes owner-only policy before every list, detail, content, and retrieval call. It also confirms that WeKnora's manual-note API accepts `publish`; the upstream API reference example using `published` is inaccurate for v0.7.2.

## Reproduce

```sh
make phase0-compose-config
make phase0-up
make phase0-runtime-check
make phase0-probe
make phase0-down
```

The runtime environment and probe report are generated under ignored `.local/`; all corpus content is synthetic.
