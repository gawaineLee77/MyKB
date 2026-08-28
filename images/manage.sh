#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
MANIFEST_DIR="$ROOT/images/manifests"
ARCHIVE_DIR="$ROOT/images/archives"

usage() {
  cat >&2 <<'EOF'
usage:
  ./images/manage.sh pull [phase3|stage1|phase0] [linux/amd64|linux/arm64]
  ./images/manage.sh build [phase3|stage1] [linux/amd64|linux/arm64]
  ./images/manage.sh list [phase3|stage1|phase0] [linux/amd64|linux/arm64]
  ./images/manage.sh save [phase3|stage1|phase0] [linux/amd64|linux/arm64]
  ./images/manage.sh load <archive.tar> [linux/amd64|linux/arm64]
EOF
  exit 2
}

profile_manifest() {
  operation=$1
  profile=$2

  case "$operation:$profile" in
    pull:phase3|pull:stage1) printf '%s\n' "$MANIFEST_DIR/stage1-external.txt" ;;
    pull:phase0) printf '%s\n' "$MANIFEST_DIR/phase0-runtime.txt" ;;
    list:phase3|save:phase3|list:stage1|save:stage1) printf '%s\n' "$MANIFEST_DIR/stage1-runtime.txt" ;;
    list:phase0|save:phase0) printf '%s\n' "$MANIFEST_DIR/phase0-runtime.txt" ;;
    *) usage ;;
  esac
}

read_manifest() {
  sed -e 's/[[:space:]]*#.*$//' -e '/^[[:space:]]*$/d' "$1"
}

require_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required" >&2
    exit 1
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "the Docker daemon is not available" >&2
    exit 1
  fi
}

normalize_platform() {
  requested=${1:-}
  if [ -z "$requested" ]; then
    requested=$(docker info --format '{{.Architecture}}')
  fi

  case "$requested" in
    amd64|x86_64|linux/amd64) printf '%s\n' 'linux/amd64' ;;
    arm64|aarch64|linux/arm64|linux/arm64/v8) printf '%s\n' 'linux/arm64' ;;
    *)
      echo "unsupported platform: $requested" >&2
      exit 2
      ;;
  esac
}

platform_archive_architecture() {
  case "$1" in
    linux/amd64) printf '%s\n' 'amd64' ;;
    linux/arm64) printf '%s\n' 'aarch64' ;;
    *) usage ;;
  esac
}

operation=${1:-}
case "$operation" in
  pull)
    require_docker
    profile=${2:-stage1}
    platform=$(normalize_platform "${3:-}")
    manifest=$(profile_manifest pull "$profile")
    read_manifest "$manifest" | while IFS= read -r image; do
      echo "Pulling $image for $platform"
      docker pull --platform "$platform" "$image"
    done
    ;;
  build)
    require_docker
    profile=${2:-stage1}
    [ "$profile" = "phase3" ] || [ "$profile" = "stage1" ] || usage
    platform=$(normalize_platform "${3:-}")
    ui_version=${MINDCREEK_UI_VERSION:-0.1.0}
    upstream_version=${WEKNORA_VERSION:-v0.7.2}
    docker buildx build \
      --platform "$platform" \
      --load \
      --tag mindcreek-ui:stage1 \
      --build-arg "MINDCREEK_UI_VERSION=$ui_version" \
      --build-arg "VITE_FRONTEND_COMMIT=weknora-$upstream_version-mindcreek-$ui_version" \
      --file "$ROOT/images/mindcreek-ui/Dockerfile" \
      "$ROOT"
    "$ROOT/scripts/build-gateway-image-offline.sh" "${platform#linux/}"
    ;;
  list)
    require_docker
    profile=${2:-stage1}
    platform=$(normalize_platform "${3:-}")
    manifest=$(profile_manifest list "$profile")
    missing=0
    while IFS= read -r image; do
      if docker image inspect --platform "$platform" "$image" >/dev/null 2>&1; then
        printf 'available  %-12s %s\n' "$platform" "$image"
      else
        printf 'missing    %-12s %s\n' "$platform" "$image"
        missing=1
      fi
    done <<EOF
$(read_manifest "$manifest")
EOF
    exit "$missing"
    ;;
  save)
    require_docker
    profile=${2:-stage1}
    platform=$(normalize_platform "${3:-}")
    manifest=$(profile_manifest save "$profile")
    architecture=$(platform_archive_architecture "$platform")
    archive="$ARCHIVE_DIR/mindcreek-$profile-$architecture.tar"
    checksum="$archive.sha256"
    archive_manifest="$archive.manifest.txt"

    if [ -e "$archive" ] || [ -e "$checksum" ] || [ -e "$archive_manifest" ]; then
      echo "refusing to overwrite existing archive: $archive" >&2
      exit 1
    fi

    missing=0
    while IFS= read -r image; do
      if ! docker image inspect --platform "$platform" "$image" >/dev/null 2>&1; then
        echo "missing image for $platform: $image" >&2
        missing=1
      fi
    done <<EOF
$(read_manifest "$manifest")
EOF
    [ "$missing" -eq 0 ] || exit 1

    mkdir -p "$ARCHIVE_DIR"
    echo "Saving $profile images for $platform to $archive"
    # Word splitting is intentional: manifests contain one whitespace-free image per line.
    # shellcheck disable=SC2046
    docker save --platform "$platform" --output "$archive" $(read_manifest "$manifest")
    cp "$manifest" "$archive_manifest"
    (
      cd "$ARCHIVE_DIR"
      archive_name=$(basename "$archive")
      checksum_name=$(basename "$checksum")
      if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$archive_name" > "$checksum_name"
      else
        shasum -a 256 "$archive_name" > "$checksum_name"
      fi
    )
    echo "Wrote $checksum and $archive_manifest"
    ;;
  load)
    require_docker
    archive=${2:-}
    [ -n "$archive" ] || usage
    [ -f "$archive" ] || {
      echo "archive not found: $archive" >&2
      exit 1
    }
    if [ -n "${3:-}" ]; then
      platform=$(normalize_platform "$3")
      docker load --platform "$platform" --input "$archive"
    else
      docker load --input "$archive"
    fi
    ;;
  *)
    usage
    ;;
esac
