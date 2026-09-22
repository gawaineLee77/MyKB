# MindCreek

MindCreek (repository: MyKB) is an upstream-first internal knowledge-base platform built around Tencent WeKnora. It uses a company default workspace, a local admin, corporate OAuth2 employee onboarding, native four-role KB/Agent management, managed models and an employee-only web assistant. R1–R5 implementation and synthetic acceptance are complete; R6 target-environment acceptance remains pending. See [R3 acceptance and limits](docs/plans/WORKSPACE_CENTRIC_R3_ACCEPTANCE.md), [R4 native UI](docs/plans/WORKSPACE_CENTRIC_R4.md), and [R5 employee assistant](docs/plans/WORKSPACE_CENTRIC_R5_ACCEPTANCE.md). See [R2 acceptance](docs/plans/WORKSPACE_CENTRIC_R2_ACCEPTANCE.md) and the [fresh-install runbook](deploy/r2/README.md).

## Documentation

Start with the [documentation index / 文档导航](docs/README.md).

- [Enterprise AMD64 deployment guide（中文）](docs/guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md): existing offline bundle, installation, models, OAuth, certificates and troubleshooting. The [historical runbook](docs/guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md) covers the old Phase 5 composition.
- [Employee website assistant（中文）](docs/guides/R5_EMPLOYEE_ASSISTANT_ZH.md): R5 enablement, publishing, employee login and embedding. Existing R4 archives need new frontend and gateway images.
- [Current capabilities and delivery status](docs/CURRENT_STATUS.md): implemented behavior and release limitations.
- [Roadmap and open work](docs/ROADMAP.md): R6 enterprise acceptance, deferred extensions, and historical items.
- [Overall design](docs/OVERALL_DESIGN.md) / [中文版](docs/OVERALL_DESIGN_ZH.md): architecture and product decisions.
- [Images and offline archives](images/README.md), [agent/MCP guide](docs/guides/AGENT_AND_MCP.md), and [upstream patch ledger](docs/UPSTREAM_PATCHES.md).

## Development checkout

```sh
git clone --recurse-submodules https://github.com/gawaineLee77/MyKB.git
cd MyKB
make phase0-check
make phase5-check
make stage1-check
make product-test-frontend
```

These are development checks, not deployment steps. Backend checks use Go 1.26; frontend checks use Node 24. See [AGENTS.md](AGENTS.md) for contributor rules. WeKnora is pinned as a clean submodule under `upstream/weknora`; product behavior lives in the gateway, adapters, configuration, and UI overlay outside that boundary.

## Current status

R1 established the workspace design. R2–R4 delivered employee/admin identity, native authorization and UI. R5 adds employee-only website publishing, disabled by default. Publication/subscription and Personal Notes remain in historical code and migrations; the native product composition rejects their retired APIs. See the [current status](docs/CURRENT_STATUS.md).

The retained historical Phase 5 server distribution runs on unmodified WeKnora v0.8.0: owner-only Personal Notes, Plain RAG and native FAQ Q&A, governed sharing/publication/subscription, authorized Web Ask and read-only MCP, managed models, corporate plain OAuth 2.0 with optional OIDC, and operational hardening.

A complete Linux AMD64 R4 **deployment/debugging bundle** and [Chinese installation guide](docs/guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md) are now available. Run its isolated database preflight on the target server first: AMD64 BM25 aborts under the current ARM-host emulator, so hybrid retrieval remains unaccepted. This package does not complete R6 production acceptance.

Engineering completion is not production approval. Optional Phase 1 Wiki/closure items and environment-specific Phase 5 acceptance remain separately tracked. The [historical archive](docs/archive/README.md) preserves prior plans and evidence; it is not a sequence of commands to run for a fresh deployment. The existing runbook describes the old runtime; R4 bundle installation and recovery instructions are available for internal debugging; target-environment acceptance remains in R6. Verify actual image versions rather than assuming they match the working tree.
