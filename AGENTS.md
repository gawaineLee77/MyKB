# Repository Guidelines

## Project Structure & Module Organization

This repository is upstream-first. `upstream/weknora` is the pinned, unmodified WeKnora submodule. Product services, adapters, configuration, migrations, and UI modules must live outside it. Phase 0 runtime files live in `deploy/phase0`, `tools/phase0`, and `testdata/phase0`. `docs/OVERALL_DESIGN.md` is authoritative, and `docs/UPSTREAM_PATCHES.md` is the downstream-change ledger.

## Build, Test, and Development Commands

Use the product-owned wrappers from the repository root:

```sh
git submodule update --init --recursive # materialize the pinned upstream
make phase0-check                       # verify boundary and design artifacts
make phase0-compose-config              # validate the local runtime profile
make phase0-up && make phase0-probe     # start and probe the synthetic baseline
make phase0-down                        # stop containers; preserve test volumes
make upstream-status                    # show tag, commit, and dirty state
make upstream-test                      # run Go, frontend, and MCP suites
```

Backend tests require Go 1.26; frontend compatibility uses Node 24; MCP tests use Python 3.12 and `uv`. Runtime fixtures are synthetic; never substitute private documents.

## Coding Style & Naming Conventions

Keep Markdown concise and use relative asset links. Use lowercase kebab-case for supporting assets. Prefer REST adapters, companion modules, and configuration over upstream edits. Never import WeKnora `internal/**` packages from product code. Record every unavoidable upstream patch in `docs/UPSTREAM_PATCHES.md`. Run `gofmt` for Go; product frontend code follows the selected TypeScript formatter and linter.

## Testing Guidelines

Every change needs proportionate tests. Prioritize authorization, tenant isolation, subscription/revocation, MCP scope, Personal Notes owner-only rules, Plain RAG fallback, profile isolation, ontology provenance, and current/candidate upstream compatibility. Use Go `*_test.go` naming and upstream frontend conventions. UI changes require screenshots; schema changes require migration and rollback/forward-upgrade verification.

## Commit & Pull Request Guidelines

Use Conventional Commits: `feat:`, `fix:`, `docs:`, `test:`, or `refactor:`. Keep commits focused. Pull requests must explain scope and rationale, link the relevant design section or issue, list verification commands, and identify permission, migration, deployment, or security impact. Include before/after screenshots for visible UI changes.

## Security & Configuration

Never commit credentials, private documents, model keys, or production configuration. Enforce authorization before retrieval, keep services private by default, and avoid sensitive document or prompt content in logs, fixtures, and screenshots.
