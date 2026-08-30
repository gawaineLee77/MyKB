# WeKnora Downstream Patch Ledger

This ledger records every product change made inside the WeKnora upstream boundary. The target state is an empty ledger: product behavior should normally live in the gateway, versioned adapter, product services, companion tables, deployment configuration, or product-owned web modules.

## Current Baseline

| Field | Value |
|---|---|
| Approved WeKnora release | v0.7.2 |
| Release commit | `3d5d8bf` |
| Ledger status | No downstream patches |
| Last reviewed | 2026-08-30 |

## Admission Rules

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
