#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
UPSTREAM="$ROOT/upstream/weknora"
MODEL="$ROOT/docs/PHASE2_SHARING_MODEL.md"
PLAN="$ROOT/docs/PHASE2_IMPLEMENTATION_PLAN.md"
EXPECTED_COMMIT="3d5d8bfcdfeeea266b292b71cea616847af28d0f"

fail() {
  echo "Phase 2 sharing-model check failed: $*" >&2
  exit 1
}

"$ROOT/scripts/verify-upstream.sh" >/dev/null

ACTUAL_COMMIT=$(git -C "$UPSTREAM" rev-parse HEAD)
[ "$ACTUAL_COMMIT" = "$EXPECTED_COMMIT" ] || fail "unexpected upstream commit $ACTUAL_COMMIT"

rg -q 'CreatorID string' "$UPSTREAM/internal/types/knowledgebase.go" || fail "upstream creator ownership seam missing"
rg -q 'ListKnowledgeBasesByTenantID' "$UPSTREAM/internal/application/repository/knowledgebase.go" || fail "tenant-wide list seam missing"
rg -q 'CREATE TABLE IF NOT EXISTS kb_shares' "$UPSTREAM/migrations/versioned/000012_organizations.up.sql" || fail "upstream kb_shares schema missing"
rg -q 'organization_id VARCHAR\(36\)' "$UPSTREAM/migrations/versioned/000012_organizations.up.sql" || fail "organization share target missing"
rg -q 'idx_kb_shares_kb_org' "$UPSTREAM/migrations/versioned/000012_organizations.up.sql" || fail "active organization-share uniqueness missing"
rg -q 'CheckTenantKBPermission' "$UPSTREAM/internal/types/interfaces/organization.go" || fail "upstream effective-permission seam missing"
rg -q 'AuditActionKBShareAdded' "$UPSTREAM/internal/types/audit_log.go" || fail "upstream share audit seam missing"

rg -q 'subject_type: user \| group \| workspace' "$MODEL" || fail "target subject model missing"
rg -q 'first enabled resolver is `subject_type=user`' "$MODEL" || fail "initial subject decision missing"
rg -q 'Existing upstream organization shares are compatibility candidates' "$MODEL" || fail "compatibility boundary missing"
rg -q '\[x\].*P2-01' "$PLAN" || fail "P2-01 is not recorded complete"

[ -z "$(git -C "$UPSTREAM" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 2 sharing-model map verified against WeKnora v0.7.2"
