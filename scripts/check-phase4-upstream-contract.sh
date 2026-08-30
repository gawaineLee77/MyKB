#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
"$ROOT/scripts/check-phase3-upstream-contract.sh" "$@"
rg -q 'SearchKnowledge' "$ROOT/services/gateway/internal/weknora/client.go"
rg -q 'AskKnowledge' "$ROOT/services/gateway/internal/weknora/client.go"
rg -q 'GetChunkExcerpt' "$ROOT/services/gateway/internal/weknora/client.go"
echo "MindCreek Phase 4 candidate-upstream retrieval and MCP adapter contract passed"
