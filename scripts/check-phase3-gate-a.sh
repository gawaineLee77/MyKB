#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE3_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE3_GATE_A.md"
MIGRATION="$ROOT/services/gateway/internal/database/migrations/000008_phase3_publications.up.sql"

fail() {
  echo "Phase 3 Gate A check failed: $*" >&2
  exit 1
}

"$ROOT/scripts/verify-upstream.sh" >/dev/null
for FILE in \
  "$MIGRATION" \
  "$ROOT/services/gateway/internal/database/migrations/000008_phase3_publications.down.sql" \
  "$ROOT/services/gateway/internal/publication/service.go" \
  "$ROOT/services/gateway/internal/subscription/service.go" \
  "$ROOT/services/gateway/internal/revision/repository.go" \
  "$ROOT/services/gateway/internal/authorization/decision.go" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

for TABLE in kb_publications kb_subscriptions kb_content_revisions kb_activity_events; do
  rg -q "CREATE TABLE mindcreek\.$TABLE" "$MIGRATION" || fail "$TABLE is missing"
done
rg -q "access_mode IN \('subscriber', 'organization_public'\)" "$MIGRATION" || fail "publication access modes are missing"
rg -q 'last_seen_revision bigint' "$MIGRATION" || fail "subscription revision state is missing"
rg -q 'row_version bigint NOT NULL DEFAULT 1' "$MIGRATION" || fail "publication concurrency is missing"
for METHOD in Publish Update Unpublish; do
  rg -q "func \(s \*Service\) $METHOD" "$ROOT/services/gateway/internal/publication/service.go" || fail "publication $METHOD service is missing"
done
for METHOD in Subscribe Unsubscribe MarkSeen; do
  rg -q "func \(s \*Service\) $METHOD" "$ROOT/services/gateway/internal/subscription/service.go" || fail "subscription $METHOD service is missing"
done
for TASK in P3-01 P3-02 P3-03 P3-04 P3-05 P3-06; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
rg -q 'Gate A.*Passed on 2026-08-27' "$PLAN" || fail "Gate A acceptance is not recorded"
rg -q '^\| Status \| Passed on 2026-08-27 \|' "$EVIDENCE" || fail "Gate A evidence status is missing"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 3 Gate A publication foundation verified"
