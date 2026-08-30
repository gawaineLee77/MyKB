# Phase 5 Implementation Plan

| Field | Value |
|---|---|
| Status | Gate A passed; Gates B–D planned |
| Planned | 2026-08-30 |
| Upstream baseline | Unmodified WeKnora v0.7.2 |
| Target | MindCreek `0.6.0-phase5` production-ready internal pilot |

## Boundaries and invariants

Phase 5 begins with a usable managed model set: one default chat (`KnowledgeQA`), embedding, and rerank model. MindCreek will reuse WeKnora's declarative built-in model mechanism rather than add another credential store. Stable model IDs and a YAML template may be committed; base URLs, API keys, rendered secret files, and production values may not. Managed credentials never reach the browser or ordinary-user API responses.

Ordinary users use the managed defaults automatically. Optional user-supplied providers appear only under Advanced Settings when `user_model_overrides` is enabled. Overrides must be owner/workspace-isolated, encrypted, allow-listed, quota-limited, and audited. If safe ownership isolation cannot be proven against the pinned upstream contract, the override capability remains disabled. An embedding change requires an explicit re-index; it is never silently applied to an existing KB.

## Task list

### A. Managed models and zero-key onboarding

- [x] **P5-01 — Managed model contract.** Define stable IDs, provider compatibility, required names/endpoints, embedding dimension, default selection, health states, quotas, and typed errors.
- [x] **P5-02 — Declarative deployment profile.** Add a product-owned built-in-model template, validated local renderer, read-only Compose mount, and secret-variable example without real values.
- [x] **P5-03 — Secret and readiness controls.** Validate required values, reject mock/test defaults outside development, test all three providers safely, and report only redacted status.
- [x] **P5-04 — Safe model facade.** Return only ID, display name, type, managed/override source, and availability to ordinary users; deny managed credential mutation outside system administration.
- [x] **P5-05 — Automatic product defaults.** Auto-select managed embedding for KB creation, chat for summaries/Ask, and rerank for retrieval and Smart Reasoning without exposing model setup in normal workflows.
- [x] **P5-06 — Advanced override lifecycle.** Add capability-gated create/test/replace/delete flows with private ownership, encrypted write-only credentials, provider/host policy, disclosure, quota, and audit.
- [x] **P5-07 — Model security matrix.** Test response/log/export redaction, cross-user access, SSRF, malformed provider data, unhealthy defaults, rotation, restart, rollback, and existing-KB embedding compatibility.

**Gate A:** A new ordinary user can create, ingest, retrieve, rerank, and ask without entering a key. No managed secret appears in client traffic, logs, probes, exports, screenshots, or Git.

**Gate A acceptance:** Passed on 2026-08-30 with the synthetic development provider profile. See [PHASE5_GATE_A.md](PHASE5_GATE_A.md). Production provider values remain an operator-supplied secret configuration.

### B. Corporate OAuth 2.0 and closed registration

- [ ] **P5-08 — Identity-provider contract.** Record issuer/endpoints, client type, scopes, claims, stable subject, group mapping, logout, and failure behavior.
- [ ] **P5-09 — Authorization flow.** Configure Authorization Code with PKCE, state, nonce where supported, exact redirect URIs, secure cookies, and trusted proxy behavior.
- [ ] **P5-10 — User and workspace provisioning.** Idempotently map the validated organization identity to MindCreek users, active workspace membership, and approved roles/groups.
- [ ] **P5-11 — Closed registration and break-glass access.** Disable self-registration, public invitations, password signup, and unintended tenant creation; retain one separately protected and audited emergency administrator.
- [ ] **P5-12 — Session lifecycle.** Implement logout, suspension, role/group change, token expiry, refresh failure, and session revocation behavior across Web and MCP.
- [ ] **P5-13 — Identity security matrix.** Cover forged state, replay, wrong issuer/audience, missing claims, disabled users, removed groups, callback errors, and direct registration routes.

**Gate B:** Only active organization identities and the controlled break-glass administrator can sign in; self-registration is unreachable.

### C. Operational hardening

- [ ] **P5-14 — TLS and network isolation.** Publish only the reverse proxy, enforce HTTPS/security headers, restrict egress, and keep application dependencies private.
- [ ] **P5-15 — Secret lifecycle.** Document creation, permissions, backup, rotation, recovery, and leak response for OAuth, model, JWT, database, Redis, and `SYSTEM_AES_KEY` secrets.
- [ ] **P5-16 — Backup and recovery.** Automate and test consistent PostgreSQL/object/config backups, restore, index rebuild, and RPO/RTO evidence.
- [ ] **P5-17 — Observability and alerts.** Add redacted structured logs, service/model health, queue/capacity/security metrics, alert thresholds, and correlation IDs.
- [ ] **P5-18 — Security and resilience tests.** Run dependency/image scans, authorization regression, rate/load tests, migration forward/rollback, failure injection, and recovery drills.

**Gate C:** The production profile passes security, load, secret-rotation, backup/restore, and failure-recovery acceptance without sensitive telemetry.

### D. Pilot and release

- [ ] **P5-19 — Pilot usability and retrieval review.** Validate zero-key onboarding with selected teams and measure multilingual retrieval quality, groundedness, latency, and cost.
- [ ] **P5-20 — Upstream compatibility.** Run inherited Gates A–D, current/candidate upstream suites, clean-copy deployment, and an empty downstream-patch review.
- [ ] **P5-21 — Phase 5 release.** Publish versioned capabilities/images, installation and upgrade runbooks, operator checklist, incident procedure, and Gate A–D evidence.

**Gate D:** MindCreek Phase 5 is reproducible from a clean checkout and approved for a controlled internal pilot.

## Deployment inputs needed later

Do not send production secrets through chat or commit them. During P5-02, the operator will place them in a local protected secret source using variables such as:

```text
MINDCREEK_MANAGED_LLM_NAME / BASE_URL / API_KEY / PROVIDER
MINDCREEK_MANAGED_EMBEDDING_NAME / BASE_URL / API_KEY / PROVIDER / DIMENSION
MINDCREEK_MANAGED_RERANK_NAME / BASE_URL / API_KEY / PROVIDER
```

The exact provider protocols and embedding dimension must be confirmed before live configuration. The implementation will include a redacted verification command that reports success or a typed failure without echoing any supplied value.
