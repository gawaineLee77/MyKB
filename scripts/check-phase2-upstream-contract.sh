#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CANDIDATE=${MINDCREEK_CANDIDATE_WEKNORA:-${1:-$ROOT/upstream/weknora}}
OVERLAY_TEMPLATE="$ROOT/tools/phase2/route_action_inventory_overlay.json"
POLICY="$ROOT/config/phase1-route-policy.json"
ACTIONS="$ROOT/config/phase2-route-actions.json"
CACHE="${GOCACHE:-$ROOT/.local/phase2-candidate-go-build}"
OVERLAY=$(mktemp "${TMPDIR:-/tmp}/mindcreek-candidate-overlay.XXXXXX")
trap 'rm -f "$OVERLAY"' EXIT HUP INT TERM

fail() {
  echo "Phase 2 upstream contract check failed: $*" >&2
  exit 1
}

[ -d "$CANDIDATE" ] || fail "candidate directory does not exist: $CANDIDATE"
[ -f "$CANDIDATE/go.mod" ] || fail "candidate is not a WeKnora source tree"
rg -q '^module github.com/Tencent/WeKnora$' "$CANDIDATE/go.mod" || fail "candidate Go module identity changed"

for CONTRACT in \
  'GET\("/:id"' \
  'GET\(""' \
  'POST\("/file"' \
  'GET\("/api-keys"' \
  'GET\("/:id/members"'; do
  rg -q "$CONTRACT" "$CANDIDATE/internal/router" || fail "required adapter route is missing: $CONTRACT"
done
for FIELD in 'json:"id"' 'json:"tenant_id"' 'json:"creator_id"'; do
  rg -q "$FIELD" "$CANDIDATE/internal/types" || fail "required knowledge-base wire field is missing: $FIELD"
done

sed "s#../../tools/#$ROOT/tools/#g" "$OVERLAY_TEMPLATE" > "$OVERLAY"
mkdir -p "$CACHE"
(
  cd "$CANDIDATE"
  MINDCREEK_ROUTE_POLICY="$POLICY" \
  MINDCREEK_PHASE2_ROUTE_ACTIONS="$ACTIONS" \
  DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}" \
  GOCACHE="$CACHE" \
  go test -overlay="$OVERLAY" ./internal/router \
    -run '^TestMindCreekPhase(1RoutePolicyCoverage|2RouteActionCoverage)$' -count=1
)

echo "MindCreek candidate-upstream adapter contract passed: $CANDIDATE"
