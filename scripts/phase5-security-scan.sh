#!/bin/sh
set -eu
umask 077

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
REPORT_DIR="$ROOT/.local/security"
MANIFEST="$ROOT/images/manifests/phase5-runtime.txt"
mkdir -p "$REPORT_DIR"
chmod 700 "$REPORT_DIR"
for stale in "$REPORT_DIR"/*.sarif.json "$REPORT_DIR"/*.scanner.log "$REPORT_DIR"/summary.json; do
  [ -e "$stale" ] && rm -f "$stale"
done

[ "${MINDCREEK_ALLOW_EXTERNAL_SCANNER:-false}" = "true" ] || {
  echo "external scan not authorized; set MINDCREEK_ALLOW_EXTERNAL_SCANNER=true after approving Docker Scout package/image metadata disclosure" >&2
  exit 2
}

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 2; }
docker scout version >/dev/null 2>&1 || { echo "Docker Scout is required for the release scan" >&2; exit 2; }

FAIL=0
while IFS= read -r image; do
  case "$image" in ""|\#*) continue ;; esac
  name=$(printf '%s' "$image" | tr '/:@' '____')
  report="$REPORT_DIR/$name.sarif.json"
  scanner_log="$REPORT_DIR/$name.scanner.log"
  rm -f "$report" "$scanner_log"

  set +e
  docker scout --debug cves --only-fixed --only-severity critical --exit-code --format sarif \
    --output "$report" "local://$image" >"$scanner_log" 2>&1
  status=$?
  set -e

  case "$status" in
    0)
      rm -f "$scanner_log"
      ;;
    2)
      rm -f "$scanner_log"
      echo "fixable critical vulnerability policy failed: $image" >&2
      FAIL=1
      ;;
    *)
      if grep -Eqi 'dial tcp|i/o timeout|no such host|could not resolve|connection refused|network is unreachable' "$scanner_log"; then
        echo "Docker Scout cannot reach its backend; check Docker Hub DNS, VPN, or proxy connectivity, then retry" >&2
      elif grep -qi 'log in with your Docker ID\|authentication required\|unauthorized' "$scanner_log"; then
        echo "Docker Scout authentication is required; sign in to Docker Desktop or run 'docker login', then retry" >&2
      else
        echo "Docker Scout could not evaluate $image (scanner exit $status)" >&2
      fi
      rm -f "$scanner_log" "$report"
      exit 2
      ;;
  esac
done < "$MANIFEST"

[ "$FAIL" -eq 0 ] || exit 1
printf '{"status":"pass","policy":"no fixable critical CVEs","reports":".local/security","secret_values":false}\n' > "$REPORT_DIR/summary.json"
chmod 600 "$REPORT_DIR"/*
echo "Phase 5 vulnerability policy passed: authorized runtime images"
