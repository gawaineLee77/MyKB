#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
UPSTREAM="$ROOT/upstream/weknora"
PHASE1_POLICY="$ROOT/config/phase1-route-policy.json"
PHASE2_ACTIONS="$ROOT/config/phase2-route-actions.json"
OVERLAY="$ROOT/tools/phase2/route_action_inventory_overlay.json"
ROUTE_ACTION_GOCACHE="${GOCACHE:-$ROOT/.local/phase2-route-go-build}"

"$ROOT/scripts/verify-upstream.sh" >/dev/null

cd "$UPSTREAM"
mkdir -p "$ROUTE_ACTION_GOCACHE"
MINDCREEK_ROUTE_POLICY="$PHASE1_POLICY" \
  MINDCREEK_PHASE2_ROUTE_ACTIONS="$PHASE2_ACTIONS" \
  DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}" \
  GOCACHE="$ROUTE_ACTION_GOCACHE" \
  go test -overlay="$OVERLAY" ./internal/router \
    -run '^TestMindCreekPhase2RouteActionCoverage$' -count=1 -v
