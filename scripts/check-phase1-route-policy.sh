#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
UPSTREAM="$ROOT/upstream/weknora"
POLICY="$ROOT/config/phase1-route-policy.json"
OVERLAY="$ROOT/tools/phase1/route_inventory_overlay.json"
ROUTE_POLICY_GOCACHE="${GOCACHE:-$ROOT/.local/phase1-route-go-build}"

"$ROOT/scripts/verify-upstream.sh" >/dev/null

cd "$UPSTREAM"
mkdir -p "$ROUTE_POLICY_GOCACHE"
MINDCREEK_ROUTE_POLICY="$POLICY" \
  DEVELOPER_DIR="${DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}" \
  GOCACHE="$ROUTE_POLICY_GOCACHE" \
  go test -overlay="$OVERLAY" ./internal/router \
    -run '^TestMindCreekPhase1RoutePolicyCoverage$' -count=1 -v
