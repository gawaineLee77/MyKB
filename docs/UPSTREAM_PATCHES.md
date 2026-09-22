# WeKnora Downstream Patch Ledger

This ledger records every product change made inside the WeKnora upstream boundary. The target state is an empty ledger: product behavior should normally live in the gateway, versioned adapter, product services, companion tables, deployment configuration, or product-owned web modules.

## Current Baseline

| Field | Value |
|---|---|
| Approved WeKnora release | v0.8.0 |
| Release commit | `1edcd54` |
| Ledger status | No downstream patches |
| Last reviewed | 2026-09-10 |

## Admission Rules

Routine implementation keeps the pinned upstream worktree clean. Before applying an exceptional patch, investigate supported extension alternatives and prepare a reviewable proposal describing the remaining invariant, proposed diff, tests, and removal condition. Meet the admission and required architecture-review rules below before applying it; a ledger entry is not approval. A phase-specific prohibition on upstream edits remains in force unless the user explicitly changes that scope.

Updating the pinned upstream release is an upgrade, not a downstream source patch. Follow the [upgrade workflow](OVERALL_DESIGN.md#137-upgrade-workflow); authorization to prepare and validate an upgrade does not by itself authorize production promotion.

A patch is allowed only when a required invariant—especially authorization before retrieval or atomic state change—cannot be enforced through a supported API, configuration, adapter, or external service. It must:

- Be a small, independently testable commit.
- Prefer a generic interface or composition-root hook over domain-algorithm changes.
- Include contract and regression tests.
- Link an upstream issue or pull request whenever generally useful.
- Define the upstream version range and an explicit removal condition.
- Pass architecture review if it touches parsing, retrieval, indexing, identity, or migrations.

Do not rewrite historical upstream migrations. Never place credentials or private data in this ledger.

## Active Patches

None.

## Pending R3 Proposals

The [R3 API-gap proposals](plans/WORKSPACE_CENTRIC_R3_UPSTREAM_GAPS.md) cover unique JWT issuance, history retrieval with a pre-query session scope (including Agent tools), and consistent Agent source selection. They apply to the pinned v0.8.0 source only; architecture review and an implementation owner remain pending. No upstream issue/PR or patch has been created or applied. Each proposal records supported alternatives, the invariant, minimal interface change, tests and removal condition. R3 uses explicit rejection where the public API is insufficient.

## Entry Template

```markdown
### PATCH-YYYY-NNN: Short title

- Status: proposed | active | upstreamed | removed
- Owner:
- First upstream version:
- Tested version range:
- Commit:
- Affected files:
- Requirement/invariant:
- Why existing extension seams are insufficient:
- Security and migration impact:
- Tests:
- Upstream issue/PR:
- Removal condition:
```

## Upgrade Review

For each candidate release, confirm whether every active patch still applies, conflicts, or has been incorporated upstream. Remove superseded patches before promoting the candidate and record the removal in Git history.
