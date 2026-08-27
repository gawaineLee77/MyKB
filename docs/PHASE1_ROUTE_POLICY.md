# Phase 1 Route and Policy Inventory

| Field | Value |
|---|---|
| Task | P1-01 |
| Status | Complete |
| Upstream | WeKnora v0.7.2 at `3d5d8bfcdfeeea266b292b71cea616847af28d0f` |
| Standard and conditional routes | 373 |
| Machine policy | `config/phase1-route-policy.json` |

## Classification result

The inventory is built from the actual Gin registration functions, not Swagger annotations. It includes 352 standard routes plus 21 conditional public embed and resource-grant routes.

| Classification | Count | Phase 1 behavior |
|---|---:|---|
| `disabled` | 101 | Gateway returns `feature.disabled`; the service or credentials are also omitted. |
| `kb_policy_controlled` | 166 | Gateway resolves the authenticated principal and every direct or indirect KB reference before forwarding. |
| `pass_through` | 106 | Gateway preserves upstream authentication and RBAC, with request limits and audit metadata. |
| Product-owned | 0 upstream routes | Exact MindCreek endpoints are handled locally and are never forwarded. |

Rules are evaluated in file order. Excluded surfaces take precedence over broader agent or infrastructure families. There is no catch-all pass-through rule: an unclassified route fails the coverage check and must receive an explicit review.

## Route-family matrix

| Route family | Classification | Reason |
|---|---|---|
| `/health` | Pass-through initially | Required for upstream dependency health; the gateway later owns its own health route. |
| `/swagger/**` | Disabled | Development surface, not part of the shared product service. |
| `/api/v1/auth/**`, `/tenants/**`, `/me/invitations/**` | Pass-through | Phase 1 preserves upstream identity and membership behavior; OAuth 2.0 and closed registration remain Phase 5. |
| `/models/**`, `/system/**`, `/vector-stores/**`, `/storage-backends/**` | Pass-through | Retained administration and infrastructure, protected by upstream RBAC. |
| Approved `/initialization/**`, `/chunker/preview`, `/skills` | Pass-through | Required for Plain RAG configuration and retained agents; excluded subpaths match earlier deny rules. |
| KB, knowledge, FAQ, chunk, Wiki, search, chat, session, message, agent, favorite, organization-share, and shared-KB families | KB-policy-controlled | These routes may reveal, select, mutate, retrieve, or derive knowledge. |
| `/files` | Disabled | A raw tenant storage path cannot be safely resolved to a Personal Notes owner at the gateway. |
| `/api/v1/knowledge-bases/:id/files` | KB-policy-controlled | The KB ID permits an owner check before image/file access. |
| `/r/:token`, `/api/v1/files/presigned*` | Disabled | Capability and presigned URLs can bypass a fresh user/KB owner check. |
| IM, WeChat, embed, MCP, Web search, data-source, and WeKnora Cloud families | Disabled | Explicitly excluded from the Phase 1 distribution. |
| Evaluation and ASR/multimodal/extraction initialization | Disabled | Cross-KB evaluation and future graph/pixel/data-analysis work are not Phase 1 capabilities. |

Planned product-owned routes include `/api/v1/capabilities/**`, `/api/v1/knowledge-spaces`, and the Note/product-profile/index-profile subresources defined in the overall design. P1-02 and later tasks will register them as exact gateway routes before proxy matching.

## Personal Notes enforcement points

Personal Notes stays capability-disabled until all groups below have negative owner/non-owner tests.

| Group | Direct and indirect checks required |
|---|---|
| KB discovery and lifecycle | Filter list/copy targets/shared lists; check detail, create mapping, update, delete, duplicate, copy, pin, activity, tags, and sharing. Sharing or publication of a Note Space is denied. |
| Source content | Resolve the parent KB for manual knowledge, uploaded knowledge, FAQ, chunks, generated questions, folders, batch operations, reparse, cancel, and move operations. |
| Retrieval | Validate every KB ID in hybrid search, knowledge search, quick chat, agent chat, and agent configuration. Empty or `all` scope must mean “all allowed,” never all workspace KBs. |
| Conversations and agents | Resolve indirect KB scope from sessions, messages, shared agents, favorites, and persisted agent configuration before returning data or starting generation. |
| Files and citations | Recheck the parent KB for preview, download, KB-scoped images, exports, saved citations, and old answer references. Raw/presigned file routes remain disabled. |
| Wiki and derived content | Apply the source Note Space owner check to every page, folder, revision, graph, search, lint, issue, revert, fix, and rebuild route. |
| Jobs and bulk requests | Resolve every referenced KB/source in copy, batch, move, parse, retry, cancel, and progress requests; reject the whole request if any item is unauthorized. |

## Gateway matching contract

For each normalized request, the future gateway must apply this order:

1. Reject malformed or ambiguous encoded paths.
2. Apply an exact product-owned route when present.
3. Apply the first matching disabled rule.
4. For KB-policy-controlled routes, resolve direct and indirect KB IDs and authorize before forwarding.
5. Forward only explicitly classified pass-through routes.
6. Deny every unclassified method/path with `route.unclassified` and log the policy decision without sensitive content.

The gateway must not trust owner IDs, workspace IDs, KB IDs, source IDs, session IDs, or agent scope supplied by the client. Indirect identifiers are resolved through the versioned adapter introduced by P1-03.

## Verification

Run:

```sh
make phase1-route-policy-check
```

The check injects a product-owned test into the pinned router with Go's build overlay, leaving `upstream/weknora` unchanged. It verifies the exact baseline, route count, uniqueness, complete classification, representative Personal Notes enforcement points, and zero downstream patches.
