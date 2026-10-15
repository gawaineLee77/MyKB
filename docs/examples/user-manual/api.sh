#!/usr/bin/env bash
# Current MindCreek JSON REST example. Multipart and SSE examples are in the manual.
set -euo pipefail

: "${MC_ORIGIN:?Set MC_ORIGIN to the product HTTPS origin}"
: "${MC_TENANT_ID:?Set MC_TENANT_ID to the numeric workspace ID}"
[[ "$MC_TENANT_ID" =~ ^[1-9][0-9]*$ ]] || { echo 'Invalid workspace ID' >&2; exit 2; }
[[ "$MC_ORIGIN" == https://* && "$MC_ORIGIN" != */ ]] || { echo 'Use an HTTPS origin without a trailing slash' >&2; exit 2; }
[[ $# -ge 2 && $# -le 3 ]] || { echo 'Usage: bash api.sh METHOD /api/v1/PATH [JSON_BODY]' >&2; exit 2; }
method=$1
path=$2
[[ "$path" == /api/v1/* && "$path" != *$'\n'* && "$path" != *$'\r'* ]] || { echo 'Use a product /api/v1/ path' >&2; exit 2; }
case "$method" in GET|POST|PUT|DELETE|PATCH) ;; *) echo 'Invalid HTTP method' >&2; exit 2 ;; esac

auth=()
case "${MC_AUTH_KIND:-key}" in
  key) : "${MC_API_KEY:?Set MC_API_KEY}"; auth=(-H "X-API-Key: $MC_API_KEY") ;;
  bearer) : "${MC_BEARER:?Set MC_BEARER}"; auth=(-H "Authorization: Bearer $MC_BEARER") ;;
  *) echo 'MC_AUTH_KIND must be key or bearer' >&2; exit 2 ;;
esac

if [[ $# -eq 3 ]]; then
  jq -e 'type == "object"' <<< "$3" >/dev/null
  if response=$(printf '%s' "$3" | curl --fail-with-body --silent --show-error \
    -X "$method" "$MC_ORIGIN$path" "${auth[@]}" -H "X-Tenant-ID: $MC_TENANT_ID" \
    -H 'Content-Type: application/json' --data-binary @-); then
    :
  else
    status=$?
    printf '%s\n' "$response" >&2
    exit "$status"
  fi
else
  if response=$(curl --fail-with-body --silent --show-error -X "$method" \
    "$MC_ORIGIN$path" "${auth[@]}" -H "X-Tenant-ID: $MC_TENANT_ID"); then
    :
  else
    status=$?
    printf '%s\n' "$response" >&2
    exit "$status"
  fi
fi
if [[ -n "$response" ]]; then
  printf '%s\n' "$response" | jq .
  printf '%s' "$response" | jq -e '.success != false and .error == null' >/dev/null
fi
