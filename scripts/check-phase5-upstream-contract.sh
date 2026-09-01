#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CANDIDATE=${MINDCREEK_CANDIDATE_WEKNORA:-${1:-$ROOT/upstream/weknora}}
"$ROOT/scripts/check-phase4-upstream-contract.sh" "$CANDIDATE"

rg -q 'BUILTIN_MODELS_CONFIG' "$CANDIDATE" || { echo "candidate lacks declarative built-in models" >&2; exit 1; }
rg -q 'OIDC_AUTH_ENABLE' "$CANDIDATE" || { echo "candidate lacks the private broker OIDC configuration seam" >&2; exit 1; }
rg -q 'OIDC_AUTH_AUTHORIZATION_ENDPOINT' "$CANDIDATE" || { echo "candidate lacks explicit OIDC broker endpoints" >&2; exit 1; }
rg -q 'DISABLE_REGISTRATION' "$CANDIDATE" || { echo "candidate lacks closed-registration configuration" >&2; exit 1; }
echo "MindCreek Phase 5 candidate-upstream model and identity contract passed: $CANDIDATE"
