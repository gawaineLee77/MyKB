#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
UPSTREAM="$ROOT/upstream/weknora"
TEMP_DIR=$(mktemp -d)
PACKAGES="$TEMP_DIR/packages.txt"
trap 'rm -rf "$TEMP_DIR"' EXIT HUP INT TERM

if [ "$(uname -s)" = "Darwin" ] && [ -d /Applications/Xcode.app/Contents/Developer ]; then
  export DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer
fi

cd "$UPSTREAM"
go list ./... | grep -v '/docreader/' >"$PACKAGES"
xargs go vet <"$PACKAGES"
xargs go test <"$PACKAGES"
go build -o "$TEMP_DIR/weknora-server" ./cmd/server

echo "WeKnora backend vet, tests, and server build passed"
