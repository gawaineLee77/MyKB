#!/usr/bin/env bash
# Current MindCreek HTTP MCP example. Requires Bash, curl and jq.
# Run explicitly; never retries questions or other requests automatically.
set -euo pipefail

: "${MC_ORIGIN:?Set MC_ORIGIN to the product HTTPS origin}"
: "${MC_TENANT_ID:?Set MC_TENANT_ID to the numeric workspace ID}"
[[ "$MC_TENANT_ID" =~ ^[1-9][0-9]*$ ]] || { echo 'Invalid workspace ID' >&2; exit 2; }
[[ "$MC_ORIGIN" == https://* && "$MC_ORIGIN" != */ ]] || { echo 'Use an HTTPS origin without a trailing slash' >&2; exit 2; }

auth=()
case "${MC_AUTH_KIND:-key}" in
  key) : "${MC_API_KEY:?Set MC_API_KEY}"; auth=(-H "X-API-Key: $MC_API_KEY") ;;
  bearer) : "${MC_BEARER:?Set MC_BEARER}"; auth=(-H "Authorization: Bearer $MC_BEARER") ;;
  *) echo 'MC_AUTH_KIND must be key or bearer' >&2; exit 2 ;;
esac

method=${1:-tools/list}
name=${2:-}
arguments=${3:-\{\}}
headers=(-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream'
  -H 'MCP-Protocol-Version: 2026-07-28' -H "Mcp-Method: $method")
case "$method" in
  tools/list)
    [[ $# -le 1 ]] || { echo 'Usage: bash mcp.sh tools/list' >&2; exit 2; }
    payload=$(jq -n '{jsonrpc:"2.0",id:1,method:"tools/list",params:{_meta:{
      "io.modelcontextprotocol/protocolVersion":"2026-07-28",
      "io.modelcontextprotocol/clientCapabilities":{}}}}')
    ;;
  tools/call)
    [[ $# -ge 2 && $# -le 3 ]] || { echo 'Usage: bash mcp.sh tools/call NAME JSON_ARGUMENTS' >&2; exit 2; }
    case "$name" in
      list_knowledge_bases|search_knowledge|get_source_excerpt|ask_knowledge_agent|wiki_search|wiki_read_page) ;;
      *) echo 'Use one of the six tools from tools/list' >&2; exit 2 ;;
    esac
    jq -e 'type == "object"' <<< "$arguments" >/dev/null
    headers+=(-H "Mcp-Name: $name")
    payload=$(jq -n --arg name "$name" --argjson args "$arguments" \
      '{jsonrpc:"2.0",id:1,method:"tools/call",params:{name:$name,arguments:$args,_meta:{
        "io.modelcontextprotocol/protocolVersion":"2026-07-28",
        "io.modelcontextprotocol/clientCapabilities":{}}}}')
    ;;
  *) echo 'Supported methods: tools/list, tools/call' >&2; exit 2 ;;
esac

if response=$(printf '%s' "$payload" | curl --fail-with-body --silent --show-error \
  "$MC_ORIGIN/mcp" "${auth[@]}" -H "X-Tenant-ID: $MC_TENANT_ID" \
  "${headers[@]}" --data-binary @-); then
  :
else
  status=$?
  printf '%s\n' "$response" >&2
  exit "$status"
fi
printf '%s\n' "$response" | jq .
# A JSON-RPC error can arrive with HTTP 200.
printf '%s' "$response" | jq -e \
  'type == "object" and has("result") and .error == null and .result.isError != true' >/dev/null
