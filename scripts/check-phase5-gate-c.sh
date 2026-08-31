#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE5_IMPLEMENTATION_PLAN.md"

fail() { echo "Phase 5 Gate C check failed: $*" >&2; exit 1; }

for file in \
  deploy/phase5/compose.production.yml \
  deploy/phase5/nginx.production.conf.template \
  scripts/phase5-production-compose.sh \
  scripts/check-phase5-production-profile.sh \
  scripts/phase5-secret-check.py \
  scripts/phase5-backup.sh \
  scripts/phase5-restore.sh \
  scripts/phase5-recovery-drill.sh \
  scripts/phase5-observability-probe.py \
  scripts/phase5-load-probe.py \
  scripts/phase5-migration-probe.py \
  scripts/phase5-failure-recovery-probe.sh \
  scripts/phase5-security-scan.sh \
  services/gateway/internal/observability/recorder.go \
  docs/PHASE5_SECRETS.md \
  docs/PHASE5_BACKUP_RECOVERY.md \
  docs/PHASE5_OBSERVABILITY.md \
  docs/PHASE5_GATE_C.md; do
  [ -f "$ROOT/$file" ] || fail "missing $file"
done

NGINX="$ROOT/deploy/phase5/nginx.production.conf.template"
COMPOSE="$ROOT/deploy/phase5/compose.production.yml"
rg -q 'listen 443 ssl' "$NGINX" || fail "TLS listener is missing"
rg -q 'Strict-Transport-Security' "$NGINX" || fail "HSTS is missing"
rg -q 'return 308 https://' "$NGINX" || fail "HTTP is not redirected"
rg -q 'WeKnora-network:[[:space:]]*$' "$COMPOSE" || fail "private backend network is missing"
rg -q 'internal: true' "$COMPOSE" || fail "backend network is not internal"
rg -q 'mindcreek-egress' "$COMPOSE" || fail "controlled provider egress is missing"
rg -q 'secret_material=excluded' "$ROOT/scripts/phase5-backup.sh" || fail "backup does not explicitly exclude secrets"
rg -q -- '--confirm-replace-current-data' "$ROOT/scripts/phase5-restore.sh" || fail "restore lacks destructive confirmation"
rg -q 'live_database_replaced.*False' "$ROOT/scripts/phase5-recovery-drill.sh" || fail "non-destructive recovery evidence is missing"
rg -q 'GET /internal/metrics' "$ROOT/services/gateway/internal/server/server.go" || fail "private metrics endpoint is missing"
rg -q 'route_class' "$ROOT/services/gateway/internal/observability/recorder.go" || fail "bounded structured request logging is missing"
rg -q 'only-fixed.*only-severity critical' "$ROOT/scripts/phase5-security-scan.sh" || fail "release vulnerability policy is missing"

for task in P5-14 P5-15 P5-16 P5-17 P5-18; do
  rg -q "\[x\].*$task" "$PLAN" || fail "$task is not recorded complete"
done
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"
"$ROOT/scripts/check-phase5-production-profile.sh" >/dev/null
echo "MindCreek Phase 5 Gate C operational hardening contract verified"
