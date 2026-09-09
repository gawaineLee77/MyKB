# Phase 5 Operations, Upgrade, and Incident Guide

## Release contents

MindCreek `0.6.0-phase5` uses unmodified WeKnora v0.7.2 and seven distinct runtime images listed in `images/manifests/phase5-runtime.txt`. Product images also carry immutable `mindcreek-ui:0.6.0` and `mindcreek-gateway:0.6.0-phase5` tags. Phase 5 adds managed zero-key models, corporate plain OAuth 2.0 with optional OIDC compatibility, closed registration, TLS/network hardening, backup/recovery, and redacted telemetry.

## Build and development start

```sh
git submodule update --init --recursive
make phase5-check
make phase5-images-pull
make phase5-images-build
make phase5-compose-config
make phase5-up
make phase5-gate-a
```

Development listens on loopback HTTP and keeps corporate identity disabled. Never use its placeholder credentials for shared service.

The vulnerability gate uses Docker Scout and may transmit runtime-image PURLs and layer digests to Docker. Run `MINDCREEK_ALLOW_EXTERNAL_SCANNER=true make phase5-security-scan` only after that external disclosure is approved; the scan does not include the source directory, runtime documents, or secret files.

## Production start

Install the TLS certificate/key and protected `.local/mindcreek.env`, set `MINDCREEK_DEPLOYMENT_ENV=production`, enable corporate identity, configure its HTTPS authorization/token/UserInfo endpoints and five-field mapping, and provide approved HTTPS model endpoints. Register the exact external origin as the corporate redirect URI:

MindCreek permits knowledge files up to 500 MiB by default. Set `MAX_FILE_SIZE_MB=500` in `.local/mindcreek.env` (or choose a smaller organization limit from 1 through 500); Compose applies it consistently to the browser, edge Nginx, Gateway, App, and Docreader. Leave `DOCREADER_GRPC_MAX_FILE_SIZE_MB` blank to inherit the same value. Migration 11 retains the historical 50-to-200 MiB transition, while migration 12 raises existing 200 MiB Plain RAG profiles to 500 MiB. Personal Notes remain unchanged.

### Scanned and image-only PDFs

The built-in PDF parser intentionally renders scanned pages as images. Plain RAG can turn those pages into searchable OCR text when an approved vision-language model is configured:

```dotenv
MINDCREEK_MANAGED_VLM_ENABLED=true
MINDCREEK_MANAGED_VLM_NAME=<vision-capable-model-name>
MINDCREEK_MANAGED_VLM_BASE_URL=https://<approved-provider>/v1
MINDCREEK_MANAGED_VLM_API_KEY=<secret>
MINDCREEK_MANAGED_VLM_PROVIDER=generic
```

The endpoint must accept image input; a text-only chat model is not sufficient. Run `make phase5-models-render`, recreate the `app` and `gateway` services, and use Settings → Model configuration to test the read-only `Vision / OCR` default. New Document RAG spaces then enable multimodal OCR automatically. For an existing knowledge base, open its settings, enable Image processing with `builtin-mindcreek-vlm`, save, then reparse each affected document. Reparse is required because previously indexed image-only chunks are not enriched retroactively. Personal Notes remain text-only, and this OCR enrichment does not enable the future PixelRAG profile.

For a PDF with a healthy selectable text layer, prefer reparsing with the MarkItDown parser instead of paying for page-by-page VLM OCR. For a large scanned-book corpus, benchmark a dedicated MinerU or PaddleOCR-VL parser before production rollout.

```text
https://<mindcreek-host>
```

Then run:

```sh
python3 scripts/phase5-secret-check.py --env-file .local/mindcreek.env
make phase5-production-compose-config
./scripts/phase5-production-compose.sh up -d
make phase5-runtime-check
make phase5-gate-b-probe
```

Only ports 80/443 on the frontend reverse proxy are published. Port 80 redirects to HTTPS; gateway, WeKnora, PostgreSQL, Redis, document reader, and model test sidecar remain private.

## Upgrade from Phase 0–4

Take a verified backup and stop the old stack without `-v`. Preserve all database/storage volumes, `SYSTEM_AES_KEY`, JWT, Redis, database, model, and identity values. For a Phase 0 volume-based deployment:

```sh
make phase0-down
./scripts/phase5-compose-from-phase0.sh config --quiet
./scripts/phase5-compose-from-phase0.sh up -d
make phase5-migration-probe
make phase5-runtime-check
```

Migration 10 adds only `mindcreek.corporate_identities` and `mindcreek.identity_audit_events`; it does not alter WeKnora's `public` schema. Never run old and new stacks against the same volumes simultaneously.

For an intentionally empty replacement that discards all old accounts, documents, and knowledge bases, follow [PHASE5_FRESH_SERVER_INSTALL.md](PHASE5_FRESH_SERVER_INSTALL.md). Do not use the fresh-reset procedure for a normal upgrade.

## Backup, rollback, and incident response

Run `make phase5-backup` before changes and `make phase5-recovery-drill` on schedule. For application rollback, stop Phase 5 and restart the previous images against preserved volumes; older releases ignore migrations 10 through 12. Do not roll back the schema unless the isolated migration test and backup retention explicitly require it.

For an incident, restrict access, preserve correlation IDs and redacted audit records, revoke exposed provider credentials, suspend affected identities, and restore the last verified bundle. Do not copy prompts, documents, answers, or secrets into tickets. Reopen access only after runtime, identity, authorization, retrieval, MCP, and observability probes pass.
