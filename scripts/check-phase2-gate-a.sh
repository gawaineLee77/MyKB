#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE2_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE2_GATE_A.md"
MIGRATION="$ROOT/services/gateway/internal/database/migrations/000006_kb_access_grants.up.sql"

fail() {
  echo "Phase 2 Gate A check failed: $*" >&2
  exit 1
}

"$ROOT/scripts/verify-upstream.sh" >/dev/null

for FILE in \
  "$MIGRATION" \
  "$ROOT/services/gateway/internal/database/migrations/000006_kb_access_grants.down.sql" \
  "$ROOT/services/gateway/internal/ownership/resolver.go" \
  "$ROOT/services/gateway/internal/grant/repository.go" \
  "$ROOT/services/gateway/internal/grant/service.go" \
  "$ROOT/services/gateway/internal/authorization/decision.go" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

rg -q 'CREATE TABLE mindcreek\.kb_access_grants' "$MIGRATION" || fail "grant table is missing"
rg -q 'kb_access_grants_active_subject_unique' "$MIGRATION" || fail "active-subject uniqueness is missing"
rg -q 'expires_at timestamptz' "$MIGRATION" || fail "grant expiry is missing"
rg -q 'revoked_at timestamptz' "$MIGRATION" || fail "grant revocation is missing"
rg -q 'revision bigint NOT NULL DEFAULT 1' "$MIGRATION" || fail "optimistic revision is missing"
rg -q 'last_audit_correlation_id' "$MIGRATION" || fail "audit correlation is missing"
for METHOD in Create List Update Revoke; do
  rg -q "func \\(s \\*Service\\) $METHOD" "$ROOT/services/gateway/internal/grant/service.go" || fail "grant $METHOD service is missing"
done
for ROLE in RoleNone RoleViewer RoleEditor RoleOwner; do
  rg -q "$ROLE[[:space:]]+Role" "$ROOT/services/gateway/internal/authorization/decision.go" || fail "authorization role $ROLE is missing"
done

for TASK in P2-03 P2-04 P2-05 P2-06; do
  rg -q "\\[x\\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
rg -q 'Gate A.*Passed on 2026-08-27' "$PLAN" || fail "Gate A acceptance is not recorded"
rg -q 'Status \| Passed on 2026-08-27' "$EVIDENCE" || fail "Gate A evidence status is missing"

[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 2 Gate A foundation verified"
