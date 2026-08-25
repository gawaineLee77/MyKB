#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
EN="$REPO_ROOT/docs/OVERALL_DESIGN.md"
ZH="$REPO_ROOT/docs/OVERALL_DESIGN_ZH.md"
DIAGRAM="$REPO_ROOT/docs/internal-kb-architecture-v0.4.png"

fail() {
  echo "design check failed: $*" >&2
  exit 1
}

for DOCUMENT in "$EN" "$ZH"; do
  test -f "$DOCUMENT" || fail "missing $DOCUMENT"
  grep -q 'v0.7.2' "$DOCUMENT" || fail "baseline missing from $DOCUMENT"
  grep -q 'GraphRAG' "$DOCUMENT" || fail "GraphRAG design missing from $DOCUMENT"
  grep -q 'PixelRAG' "$DOCUMENT" || fail "PixelRAG design missing from $DOCUMENT"
  grep -q 'Semantica' "$DOCUMENT" || fail "ontology reference missing from $DOCUMENT"
  FENCES=$(grep -c '^```' "$DOCUMENT")
  test $((FENCES % 2)) -eq 0 || fail "unbalanced code fences in $DOCUMENT"
done

EN_HEADINGS=$(grep -Ec '^#{2,4} ' "$EN")
ZH_HEADINGS=$(grep -Ec '^#{2,4} ' "$ZH")
test "$EN_HEADINGS" -eq "$ZH_HEADINGS" || fail "English/Chinese heading counts differ"

test -f "$DIAGRAM" || fail "architecture diagram is missing"
file "$DIAGRAM" | grep -q 'PNG image data' || fail "architecture diagram is not a PNG"

echo "Design documents verified: $EN_HEADINGS aligned headings"
