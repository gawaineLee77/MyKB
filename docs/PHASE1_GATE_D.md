# Phase 1 Gate D — Plain RAG

| Field | Result |
|---|---|
| Status | Passed on 2026-08-26 |
| RAG engine | WeKnora v0.7.2 through a thin adapter |
| Indexing | Vector plus keyword, versioned preset v1 |
| Future dependencies | Graph, pixel, Wiki, and Neo4j disabled |

## Verified behavior

MindCreek owns the mode, policy, preset, limits, and UI; it does not fork or replace WeKnora's RAG engine. A stored `plain` profile deterministically reproduces chunking, local storage, model selection, indexing, and retrieval settings. The gateway validates this profile before accepting a document and then delegates parsing, chunking, indexing, retry, and cancellation upstream.

The Stage 2 RAG workspace supports Markdown, text, PDF, Word, spreadsheet, presentation, CSV, HTML, JSON, and XML files up to 50 MiB. It displays queue/processing/final states, failure details, retry, cancel, native advanced view, and normal chat.

The live Gate D probe generates synthetic Markdown, PDF, DOCX, and XLSX fixtures. All reach `completed`; retry and cancel are exercised. Exact English and Chinese sentinel queries pass hybrid retrieval and normal chat. Returned references are authorized and open successfully through both document and chunk APIs.

Browser evidence for a completed synthetic upload and the managed Plain RAG preset is recorded in the [Phase 1 UI evidence](PHASE1_UI_EVIDENCE.md).

## Reproduce

```sh
make phase1-gateway-build-offline
make phase1-up
make phase1-gate-d
```

Results are written to `.local/phase1-gate-d-report.json`. The deterministic mock OpenAI sidecar is test-only; deployments must configure approved embedding and chat models.
