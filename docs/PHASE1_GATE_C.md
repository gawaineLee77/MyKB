# Phase 1 Gate C — Personal Notes Functions

| Field | Result |
|---|---|
| Status | Passed on 2026-08-26 |
| Upstream | Unmodified WeKnora v0.7.2 |
| Note formats | UTF-8 `.md` and `.txt` only |
| Recovery | Immutable revisions plus optimistic concurrency |

## Verified behavior

MindCreek creates Note Spaces atomically through the product gateway and records an owner-only `notes_plain` profile. The Stage 2 workspace provides note list, Markdown editor, safe text preview, import, delete, version preview, and restore.

The product service enforces 64 KiB per note, 500 notes, and a 2 MiB pilot corpus limit before delegating to WeKnora manual knowledge APIs. PDF/DOCX input, invalid UTF-8, oversize content, and stale version edits stop without upstream mutation. Restoring an old revision creates a newer version and retains all history.

The live probe verifies owner CRUD, non-owner denial, import policy, quotas, stale-edit conflict, revision preview/restore, and asynchronous deletion.

Browser evidence for the creation wizard and saved Markdown workspace is recorded in the [Phase 1 UI evidence](PHASE1_UI_EVIDENCE.md).

## Reproduce

```sh
make phase1-gateway-build-offline
make phase1-up
make phase1-gate-c
```

The probe uses synthetic users and content and writes `.local/phase1-gate-c-report.json`.
