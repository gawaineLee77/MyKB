# WeKnora and Dify Comparison

| Field | Value |
|---|---|
| Status | Architecture decision support |
| Date | 2026-08-05 |
| Product goal | Internal personal, shared, and subscribed knowledge bases |
| Recommendation | Use WeKnora as the base platform |

## Executive Summary

WeKnora and Dify overlap in RAG, agents, model integration, APIs, observability, and self-hosting, but they are designed for different products.

- **WeKnora is a knowledge platform.** Its main objects are knowledge bases, documents, FAQs, Wiki pages, indexes, citations, and knowledge-focused agents.
- **Dify is an AI application platform.** Its main objects are applications, prompts, workflows, tools, agents, plugins, and deployable APIs.

For this repository's goal—allowing internal users to build, share, publish, subscribe to, and query knowledge bases—WeKnora is the closer foundation. Dify is preferable when the primary requirement is visual business-process orchestration or building many unrelated AI applications.

## Capability Comparison

| Area | WeKnora | Dify |
|---|---|---|
| Primary purpose | Enterprise knowledge management and reasoning | General AI application development |
| Main experience | Ready-made knowledge portal | Low-code application and workflow studio |
| Knowledge model | Documents, FAQ, chunks, graph, and structured Wiki | Datasets, chunks, metadata, and retrieval indexes |
| Persistent Wiki | Native agent-generated, interlinked Markdown Wiki | Not a native core product concept |
| Knowledge graph | Wiki graph and GraphRAG-oriented capabilities | Can be assembled with workflows or plugins |
| Ingestion | Upload-specific parsing, OCR/VLM, chunking, reprocessing, and source synchronization | Visual Knowledge Pipelines with configurable data-source and processing nodes |
| Retrieval | Vector, keyword, FAQ, hybrid, graph, and Wiki retrieval | Configurable dataset retrieval and Knowledge Pipeline output |
| Agents | Quick RAG, ReAct reasoning, custom agents, citations | ReAct/function-calling agents and customizable agent strategies |
| Workflow orchestration | Limited and knowledge-focused | Strong visual branching, iteration, variables, tools, and triggers |
| Collaboration | Multi-workspace RBAC, per-KB ownership, shared resources, and audit | Collaborative workspaces; some organization features depend on edition |
| KB subscription | Not native; requires development | Not native; requires development |
| Extensibility | REST API, MCP, model/vector/storage adapters | Broad tool, model, datasource, trigger, endpoint, and agent-strategy plugins |
| Operations | Knowledge ingestion queues, retrieval evaluation, Langfuse tracing | Application/workflow logs, LLMOps, tracing, and plugin runtime |
| License | MIT | Dify Open Source License, based on Apache 2.0 with additional conditions |

## Architectural Difference

WeKnora treats knowledge as the product:

```text
Sources
  ├── document and FAQ indexes
  ├── keyword/vector retrieval
  ├── knowledge graph
  └── persistent structured Wiki
                ↓
        knowledge-focused agent
```

Dify treats knowledge as one component of an AI application:

```text
Data sources
      ↓
Knowledge Pipeline
      ↓
Dataset and retrieval
      ↓
Workflow, chatbot, agent, or API
```

WeKnora's important knowledge-specific advantage is the combination of conventional RAG with a persistent, human-browsable Wiki. Raw documents can remain the source of truth while agents create reusable pages, relationships, summaries, and graph structures. Dify's Knowledge Pipeline is more flexible for visually configuring ingestion, but its normal output remains a retrievable dataset rather than a continuously maintained Wiki.

This is a meaningful advance for knowledge management, not a universal replacement for Dify. Persistent Wiki generation also costs more during ingestion and requires provenance, review, regeneration, and stale-content controls.

## Where WeKnora Is Stronger

- Ready-to-use KB and document-management interface
- Document, FAQ, and Wiki knowledge-base types
- Persistent Wiki generation and graph browsing
- Per-document processing configuration and reprocessing
- Knowledge citations and retrieval evaluation
- Per-KB ownership, workspace RBAC, and activity auditing
- Existing shared-space and cross-workspace foundations
- MIT license, which is convenient for a customized internal fork
- Closer alignment with the planned `My KBs`, `Shared`, `Subscribed`, and `Discover` experience

## Where Dify Is Stronger

- Visual workflows with branches, loops, variables, and multi-step state
- Building chatbots, generators, agents, and automation applications beyond knowledge management
- Larger model, tool, datasource, trigger, and plugin ecosystem
- Prompt experimentation and application lifecycle management
- Publishing AI applications through reusable APIs
- Connecting knowledge retrieval to external business actions
- Broader platform community and ecosystem maturity

## Fit for This Repository

| Requirement | Preferred base | Reason |
|---|---|---|
| Personal KB ownership | WeKnora | Per-KB ownership and knowledge-oriented UI already exist. |
| Explicit KB sharing | WeKnora | Existing RBAC and sharing provide a closer starting point. |
| Publication catalog | WeKnora plus custom domain | Publications naturally reference existing KB objects. |
| Subscribe/unsubscribe | WeKnora plus custom domain | Live subscriptions can extend KB access without copying indexes. |
| Unified authorized agent | WeKnora | Retrieval and agents are already organized around KB scope. |
| Persistent Wiki and graph | WeKnora | Native product capability. |
| Visual business automation | Dify | Workflow orchestration is Dify's principal advantage. |
| Many unrelated AI applications | Dify | Its application builder and plugin architecture are more appropriate. |

## Decision

Use **WeKnora v0.7.1** as the initial upstream baseline and create a focused web-only distribution.

Keep:

- Authentication, workspaces, RBAC, and audit
- Knowledge, document, FAQ, Wiki, retrieval, and agent services
- Web interface, REST API, model administration, storage, and processing workers

Remove or disable:

- WeChat Mini Program
- CLI
- IM channels
- Public embed, browser extension, web search, MCP, ASR, and data-analysis features unless later requirements justify them

Add:

- Private-by-default KB visibility
- Explicit Viewer and Editor grants
- Internal publication catalog
- Live, read-only subscriptions without document or embedding duplication
- Authorization scope resolution across owned, shared, and subscribed KBs

## Possible Future Dify Integration

Dify should not be part of the initial core deployment. If future users need visual workflows, it can run as a separate automation layer:

```text
Dify workflow
      ↓ scoped service API
WeKnora authorization and knowledge API
      ↓
Authorized search or agent result
```

WeKnora remains the knowledge system of record and enforces KB permissions. Dify receives only scoped API credentials and must never connect directly to WeKnora's database, object storage, or vector index.

## Licensing Note

WeKnora is MIT licensed. Dify uses its own Apache-derived license with additional conditions. Any future code reuse or combined distribution should receive a license review; API-level integration keeps the systems and licenses more clearly separated.

## References

- [WeKnora repository and feature overview](https://github.com/Tencent/WeKnora)
- [WeKnora v0.7.1 release](https://github.com/Tencent/WeKnora/releases/tag/v0.7.1)
- [Dify repository and feature overview](https://github.com/langgenius/dify)
- [Dify Knowledge Pipeline announcement](https://github.com/langgenius/dify/discussions/26138)
- [Dify plugin types](https://docs.dify.ai/en/develop-plugin/getting-started/choose-plugin-type)
