# Repository Guidelines

## Project Structure & Module Organization

This repository is upstream-first. Keep the pinned `upstream/weknora` submodule clean during routine implementation. Product services, adapters, configuration, migrations, and UI modules live outside it. Phase 0 runtime files live in `deploy/phase0`, `tools/phase0`, and `testdata/phase0`. `docs/OVERALL_DESIGN.md` is authoritative; `docs/UPSTREAM_PATCHES.md` governs exceptional upstream patches.

## Autonomy & Task Completion

Once the user authorizes a task range, Gate, or Phase, implement and verify small steps in dependency order, then continue automatically within that scope. Progress updates are not approval gates. Reuse still-valid authorization for the same scope and action; do not repeatedly request confirmation. Ask only for a material choice that cannot reasonably be inferred, an explicit review/approval gate, or missing execution permission. Pause only the affected steps and continue independent authorized work. Report incomplete work and pending checks accurately; implementation authority does not imply deployment, publication, or other external-action authority.

## Build, Test, and Development Commands

Use the product-owned wrappers from the repository root:

```sh
git submodule update --init --recursive # materialize the pinned upstream
make phase0-check                       # verify boundary and design artifacts
make phase0-compose-config              # validate the local runtime profile
make phase1-gateway-test                # test the product gateway and adapters
make phase5-check                       # inherited product contracts and configuration
make stage1-check                       # verify product UI overlay and upstream boundary
make product-test-frontend              # test, type-check, and build the overlaid UI
make phase0-up && make phase0-probe     # start and probe the synthetic baseline
make phase0-down                        # stop containers; preserve test volumes
make upstream-status                    # show tag, commit, and dirty state
make upstream-test                      # run Go, frontend, and MCP suites
```

Backend tests require Go 1.26; frontend compatibility uses Node 24; MCP tests use Python 3.12 and `uv`. Runtime fixtures are synthetic; never substitute private documents.

## Coding Style & Naming Conventions

Keep Markdown concise and use relative asset links. Use lowercase kebab-case for supporting assets. Prefer REST adapters, companion modules, and configuration over upstream edits. Never import WeKnora `internal/**` packages from product code. Investigate supported alternatives and prepare a reviewable proposal before an exceptional upstream patch; meet the ledger's admission and required architecture-review rules before applying it. Recording a patch does not approve it. A pinned-version upgrade is distinct from a source patch; production promotion retains explicit approval. Run `gofmt` for Go; product frontend code follows the selected TypeScript formatter and linter.

## Testing Guidelines

Every change needs proportionate verification. Documentation-only edits need content, link, and formatting checks, not new tests. Prioritize authorization, tenant isolation, subscription/revocation, MCP scope, Personal Notes owner-only rules, Plain RAG fallback, profile isolation, ontology provenance, and current/candidate upstream compatibility. Use Go `*_test.go` naming and upstream frontend conventions. Schema changes require migration and rollback/forward-upgrade verification. Upstream CI success does not replace the product checks and acceptance criteria relevant to the change.

Visible UI changes require before/after screenshots. If browser access is unavailable, continue implementation and executable checks, record visual verification as pending, and honor any applicable user-approved deferral. Missing screenshots must not be reported as passing release acceptance without an explicit deferral or waiver.

## Commit & Pull Request Guidelines

Use Conventional Commits: `feat:`, `fix:`, `docs:`, `test:`, or `refactor:`. Keep commits focused. Pull requests must explain scope and rationale, link the relevant design section or issue, list verification commands, and identify permission, migration, deployment, or security impact. Report screenshot evidence or its pending/deferred status under Testing Guidelines.

## Security & Configuration

Never commit credentials, private documents, model keys, or production configuration. Enforce authorization before retrieval, keep services private by default, and avoid sensitive document or prompt content in logs, fixtures, and screenshots.
