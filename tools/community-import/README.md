# Community Import Tool

Product-owned, dependency-free Python 3.12+ tool for corporate article APIs, HTML/S3 images, document bodies, Markdown, attachments, and MindCreek ingestion. Runs on Linux/macOS (Windows: WSL). It does not enable the upstream CLI or connector service.

Start with the [Chinese operator guide](../../docs/guides/COMMUNITY_IMPORT.md).

```sh
python3 tools/community-import/import_posts.py init
python3 tools/community-import/import_posts.py --help
make community-import-test
# Optional: choose a supported interpreter explicitly.
make community-import-test COMMUNITY_IMPORT_PYTHON=python3.12
```

Commands: `fetch`, `prepare`, `upload`, `check`, `status`, `run`, and explicit `prune --apply`. Endpoints remain placeholders in `config.example.json`; credentials are read only from environment-backed headers. Default output is ignored `.local/` storage with private permissions.

R3 uses native `/knowledge-bases/:id/knowledge/file`, `/knowledge/:id`, and `/knowledge/:id/reparse` through the product gateway. It no longer calls the retired product ingestion endpoint. The original credential must have native upload/write access; read access alone is insufficient.

Optional `create-kb --name "Community"` creates a native document KB and prints its ID. Put that ID into `mindcreek.knowledge_base_id` before upload. It saves creation intent before the write, reuses a confirmed ID on repeated runs, and reconciles uncertain results by an installation-specific description marker. An unresolved write is never blindly repeated. Managed defaults are supplied by the R3 gateway; no employee model key is required.

`status` reports cached upload counts for the configured destination without network access. Exit codes: 0 success, 1 failed preparation/upload/parsing, 2 pending or not uploaded, 130 interrupted. Use `check` to refresh remote state. A prepared offline example correctly reports code 2 until uploaded.

The test suite uses only synthetic local HTTP services. Real corporate API integration and VLM/retrieval quality must be verified against an approved sample after operator configuration. No local or remote content is automatically deleted by the normal import workflow.
