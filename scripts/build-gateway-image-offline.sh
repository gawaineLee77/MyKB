#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TARGET_ARCH=${1:-$(go env GOARCH)}
BUILD_DIR="$ROOT/.local/gateway-build"
GO_CACHE="$ROOT/.local/gateway-go-build"
OUTPUT="$BUILD_DIR/mindcreek-gateway-linux-$TARGET_ARCH"
VERSION=${MINDCREEK_VERSION:-0.4.0-phase3}

case "$TARGET_ARCH" in
  amd64|arm64) ;;
  *) echo "unsupported gateway target architecture: $TARGET_ARCH" >&2; exit 2 ;;
esac

mkdir -p "$BUILD_DIR" "$GO_CACHE"
(
  cd "$ROOT/services/gateway"
  CGO_ENABLED=0 GOOS=linux GOARCH="$TARGET_ARCH" GOCACHE="$GO_CACHE" GOPROXY=off GOSUMDB=off \
    go build -trimpath -ldflags "-s -w -X main.productVersion=$VERSION" \
    -o "$OUTPUT" ./cmd/gateway
)

docker build \
  --platform "linux/$TARGET_ARCH" \
  --build-arg "TARGETARCH=$TARGET_ARCH" \
  --build-arg "MINDCREEK_VERSION=$VERSION" \
  --file "$ROOT/images/mindcreek-gateway/Dockerfile.offline" \
  --tag "mindcreek-gateway:phase3" \
  "$ROOT"

# Keep the earlier local tags as compatibility aliases for existing ignored
# .local environment files; new deployments use the Phase 3 tag.
docker tag mindcreek-gateway:phase3 mindcreek-gateway:phase2
docker tag mindcreek-gateway:phase3 mindcreek-gateway:phase1
echo "Built mindcreek-gateway:phase3 for linux/$TARGET_ARCH without a runtime base image"
