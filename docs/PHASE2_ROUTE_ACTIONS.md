# Phase 2 Route-Action Inventory

| Field | Value |
|---|---|
| Task | P2-02 |
| Status | Complete |
| Verified | 2026-08-27 |
| Baseline | WeKnora v0.7.2 (`3d5d8bfcdfeeea266b292b71cea616847af28d0f`) |
| Machine policy | `config/phase2-route-actions.json` |

## Classification result

The inventory starts from all 166 routes classified as `kb_policy_controlled` by Phase 1. Rules match both HTTP method and normalized Gin path. Every route must match exactly one rule; no default or method-only fallback exists.

| Action | Routes | Required KB permission |
|---|---:|---|
| `discover` | 5 | Return only the caller's Owner/Editor/Viewer result set. |
| `read` | 73 | Owner, Editor, or Viewer. |
| `edit_content` | 45 | Owner or Editor; Personal Notes remain owner-only. |
| `configure` | 10 | Owner by default; P2-11 may allow an explicit limited Editor subset. |
| `manage_grants` | 32 | Owner only; a KB grant never authorizes organization administration. |
| `delete` | 1 | Owner only; this action means deleting the KB itself. |

The broad Phase 1 policy intentionally includes agents, sessions, favorites, shared-resource lists, and organization routes because they may expose or select KB scope. Phase 2 keeps those routes classified: session and favorite changes are `read` relative to KB permission, agent configuration is `configure`, and upstream organization/share control planes are `manage_grants` so Viewer/Editor grants cannot unlock them.

## Important method exceptions

HTTP method alone is not an authorization action. These POST operations require readable KB scope rather than Editor permission:

- `/knowledge-search`
- `/knowledge-chat/:session_id`
- `/agent-chat/:session_id`
- `/knowledge-bases/:id/hybrid-search`
- FAQ search and similar-question lookup
- `/messages/search`
- user-owned session, attachment, suggestion, stop, pin, and title operations

Conversely, deleting a document, chunk, FAQ entry, tag, or Wiki artifact is `edit_content`; only `DELETE /knowledge-bases/:id` is the owner-only `delete` action. Share listing is also `manage_grants`, preventing a Viewer from enumerating other grantees.

## Gateway contract

The future Phase 2 gateway decision order is:

1. Classify the normalized method and path.
2. Resolve every direct and indirect KB reference.
3. Resolve Owner/Editor/Viewer/None for the authenticated principal.
4. Apply the action's minimum permission and any narrower product-mode rule.
5. Proxy only after authorization; otherwise return a non-disclosing denial.

Product-owned grant endpoints will use `manage_grants` but are not counted in the 166 upstream routes. Personal Notes always apply their owner-only rule after action classification.

## Verification

Run:

```sh
make phase2-route-actions-check
```

The test injects product-owned inventory tests into the pinned router using a Go build overlay. It verifies the upstream identity, Phase 1 controlled-route count, unique action match, representative read-like POST exceptions, permission-sensitive samples, and an unchanged upstream worktree.
