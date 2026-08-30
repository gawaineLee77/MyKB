# Phase 4 Gate D — Release and Compatibility

| Field | Result |
|---|---|
| Status | Passed on 2026-08-30 |
| Scope | P4-17 and P4-18 |
| Product release | MindCreek `0.5.0-phase4` |
| Upstream | WeKnora v0.7.2, unmodified |

## Release contract

Phase 4 selects `config/phase4-capabilities.json`, `mindcreek-ui:phase4`, and `mindcreek-gateway:phase4`. The seven-image manifest remains suitable for offline AMD64 or ARM64 transfer. The WeKnora app and gateway remain private; only the frontend is published to the host.

The candidate-upstream contract verifies the versioned retrieval, chat, session, chunk, library, publication, and subscription seams. The clean-copy check reconstructs the candidate in a fresh clone, initializes the pinned submodule, compiles every gateway package, runs Phase 4 security tests, verifies the overlay, and validates Compose without relying on untracked build state.

## Acceptance evidence

- All MindCreek gateway packages and inherited Phase 0–3 policy/security contracts pass.
- The pinned WeKnora backend suite, vet, and server build pass; the upstream frontend passes 341 tests, type-check, and production build; the upstream MCP server passes 23 tests.
- The Node 24 product-image build passes 346 frontend tests, type-check, and production build.
- Live Gate B records Recall@5 `1.0`, grounded quick/reasoning answers, and immediate authorization loss. Live Gate C verifies all six tools and eight redacted audit events.
- All seven ARM64 runtime images are present. AMD64 uses the same pinned manifest and explicit cross-platform build targets.
- The downstream patch ledger remains empty.

## Reproduce

```sh
make phase4-check
make phase4-gate-b
make phase4-gate-c
make phase4-gate-d
```

Live probes and Docker image builds remain explicit. Their reports and archives are generated artifacts and are not committed as source.
