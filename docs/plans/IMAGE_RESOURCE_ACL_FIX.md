# Image resource authorization repair

The 20261019 gateway left `kbguard.AuthorizeResource` unwired and rejected every message-file read. This repair restores protected images through the unchanged pinned WeKnora v0.8.0. It does not widen raw storage routes or infer ownership from editable Markdown, `image_info`, or native attachment lifecycle bindings.

## Architecture

Public REST can read processing spans by document but cannot resolve a resource to its source document. The product adapter therefore uses fixed, parameterized, read-only queries against the pinned native `public.resources`, `knowledge_processing_spans`, `knowledges`, `knowledge_bases`, `messages`, and `sessions` schemas. No native writes, triggers, internal Go imports, or upstream patches are introduced.

A live resource must match the authenticated workspace, exact canonical physical location and location hash, image kind, expiry and deletion state. Only the native exports image namespace is supported by the existing KB file proxy. Multimodal worker image subspans must have the correct same-document/attempt stage parent. Their input comes from newly stored parser image bytes; existing provider/resource links are skipped by ImageResolver. Native attachment bindings are excluded because manual-document copying can create them from arbitrary same-workspace links.

Human workspace Owner/Admin use the intended tenant-wide KB exception; creator `Manage` and API-key projected identities do not count. A private chat/temporary-document image without independently verified KB provenance is excluded, including attachments of deleted sessions. All other KB reads require a live owning document in the requested KB and fresh KB ACL. Message reads retain session ownership and scope checks, then require a currently readable KB in that scope or a server-stored user image attachment of that message. The upstream handler independently checks tenant and storage before returning bytes.

Migration 000022 stores only explicit local-operator approvals, bound to the resource identity and physical-location fingerprint. The local CLI checks active same-workspace document/resource state. It has no web endpoint; repeated identical approvals preserve original evidence. A populated down migration refuses data loss. Parser-backed resources resolve automatically without a bulk content rewrite. Unverifiable resources remain closed to ordinary users until independently reviewed.

Legacy registered provider references are rewritten to canonical opaque handles before existing response redaction. This translation grants no access. Missing old locators do not erase the surrounding paragraph. Original file metadata remains redacted. Response cache controls and browser cache revalidation prevent reuse of old fetch results after a permission refresh; already delivered/copied bytes cannot be revoked.

## Verification and delivery

Required checks: current principal vs creator/key; cross-workspace/path substitution; fabricated content/bindings; live/deleted documents; revoked access; private/public/shared KBs; session ownership and scope; registered legacy aliases; stale approval fingerprint; migration up/repeat/down/forward; no-store and stale browser requests.

Prepare a new immutable Linux AMD64 complete offline package and test upgrade from `current-20261019-d3fafdaf`; retain R4/1009 compatibility through their existing upgrade checks. Synthetic test data only. The final package record supplies commands, hashes and completed/pending acceptance. The prior archive is not modified. Enterprise deployment is not performed by this development task.

Operator instructions: [图片权限修复与升级](../guides/IMAGE_PERMISSION_RECOVERY_ZH.md).
