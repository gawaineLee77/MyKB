#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
"$ROOT/scripts/check-phase2-upstream-contract.sh" "$@"
echo "MindCreek Phase 3 candidate-upstream contract passed without new upstream seams"
