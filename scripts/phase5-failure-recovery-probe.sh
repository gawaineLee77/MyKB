#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE="$ROOT/scripts/phase5-compose.sh"
BASE_URL=${MINDCREEK_BASE_URL:-http://127.0.0.1:18080}
REPORT="$ROOT/.local/phase5-failure-recovery-report.json"
STARTED=$(date +%s)

recover() {
  $COMPOSE up -d app gateway frontend >/dev/null 2>&1 || true
}
trap recover EXIT HUP INT TERM
$COMPOSE stop -t 20 app >/dev/null

RESPONSE=$(mktemp "${TMPDIR:-/tmp}/mindcreek-failure.XXXXXX")
trap 'rm -f "$RESPONSE"; recover' EXIT HUP INT TERM
STATUS=$(curl --silent --show-error --output "$RESPONSE" --write-out '%{http_code}' "$BASE_URL/api/v1/auth/oidc/config" || true)
[ "$STATUS" = "502" ] || { echo "application failure did not produce a bounded 502 response: $STATUS" >&2; exit 1; }
if rg -qi '(postgres://|redis://|api[_-]?key|client[_-]?secret|password)' "$RESPONSE"; then
  echo "failure response disclosed sensitive configuration" >&2
  exit 1
fi

recover
for attempt in $(seq 1 30); do
  if "$ROOT/scripts/phase5-runtime-check.sh" >/dev/null 2>&1; then break; fi
  [ "$attempt" -lt 30 ] || { echo "runtime did not recover" >&2; exit 1; }
  sleep 2
done
FINISHED=$(date +%s)
DURATION=$((FINISHED - STARTED))
python3 - "$REPORT" "$DURATION" <<'PY'
import json, os, sys
path, duration = sys.argv[1], int(sys.argv[2])
with open(path, "w", encoding="utf-8") as handle:
    json.dump({"status": "pass", "injected_failure": "application stopped", "public_response": 502, "sensitive_response": False, "recovery_seconds": duration}, handle, indent=2)
    handle.write("\n")
os.chmod(path, 0o600)
PY
rm -f "$RESPONSE"
trap - EXIT HUP INT TERM
echo "Phase 5 failure recovery passed: bounded 502 and healthy restart in ${DURATION}s"
