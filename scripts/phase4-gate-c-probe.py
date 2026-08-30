#!/usr/bin/env python3
"""Run the live Phase 4 hosted MCP protocol and authorization matrix."""

from __future__ import annotations

import argparse
import json
import os
import runpy
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
SUPPORT = runpy.run_path(str(ROOT / "scripts/phase4_probe_support.py"))
Client = SUPPORT["Client"]
APIError = SUPPORT["APIError"]
create_fixture = SUPPORT["create_fixture"]
wait_for_gateway = SUPPORT["wait_for_gateway"]
MODERN_PROTOCOL = "2026-07-28"


def rpc(
    client: Any,
    token: str,
    method: str,
    params: dict[str, Any],
    correlation: str,
    *,
    name: str = "",
    modern: bool = True,
    extra_headers: dict[str, str] | None = None,
) -> tuple[int, dict[str, Any]]:
    request_params = dict(params)
    headers = {
        "Accept": "application/json, text/event-stream",
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "X-Request-ID": correlation,
    }
    if modern:
        request_params["_meta"] = {
            "io.modelcontextprotocol/protocolVersion": MODERN_PROTOCOL,
            "io.modelcontextprotocol/clientCapabilities": {},
        }
        headers["MCP-Protocol-Version"] = MODERN_PROTOCOL
        headers["Mcp-Method"] = method
        if name:
            headers["Mcp-Name"] = name
    if extra_headers:
        headers.update(extra_headers)
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": request_params}).encode("utf-8")
    request = urllib.request.Request(client.base_url + "/mcp", data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            status, raw = response.status, response.read(4 * 1024 * 1024 + 1)
    except urllib.error.HTTPError as error:
        status, raw = error.code, error.read(4 * 1024 * 1024 + 1)
    if len(raw) > 4 * 1024 * 1024:
        raise RuntimeError("MCP returned an oversized response")
    return status, json.loads(raw) if raw else {}


def tool_call(client: Any, token: str, name: str, arguments: dict[str, Any], correlation: str) -> dict[str, Any]:
    status, body = rpc(
        client,
        token,
        "tools/call",
        {"name": name, "arguments": arguments},
        correlation,
        name=name,
    )
    if status != 200 or body.get("error"):
        raise RuntimeError(f"MCP {name} failed: {status}/{body.get('error')}")
    return body["result"]["structuredContent"]


def expect_rpc_error(body: dict[str, Any], code: int) -> None:
    if int(body.get("error", {}).get("code", 0)) != code:
        raise RuntimeError(f"MCP error was {body.get('error')}, expected {code}")


def audit_count(prefix: str) -> int:
    values: dict[str, str] = {}
    for raw in (ROOT / ".local/mindcreek.env").read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip().strip('"').strip("'")
    escaped = prefix.replace("'", "''")
    sql = (
        "SELECT count(*) FROM mindcreek.agent_operation_audit_events "
        f"WHERE client_kind='mcp' AND correlation_id LIKE '{escaped}%'"
    )
    result = subprocess.run(
        ["docker", "exec", "MindCreek-postgres", "psql", "-U", values["DB_USER"], "-d", values["DB_NAME"], "-Atqc", sql],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError("unable to verify MCP audit events")
    return int(result.stdout.strip())


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--report", default=str(ROOT / ".local/phase4-gate-c-report.json"))
    args = parser.parse_args()
    client = Client(args.base_url)
    wait_for_gateway(client, args.health_timeout)
    fixture = create_fixture(client, "phase4-mcp")
    bob_token, alice_token = fixture["bob_token"], fixture["alice_token"]
    kb_ids, sentinels = fixture["kb_ids"], fixture["sentinels"]
    prefix = f"phase4-mcp-{fixture['nonce']}"

    status, discovery = rpc(client, bob_token, "server/discover", {}, f"{prefix}-discover")
    if status != 200 or MODERN_PROTOCOL not in discovery.get("result", {}).get("supportedVersions", []):
        raise RuntimeError("modern MCP discovery failed")
    status, listed = rpc(client, bob_token, "tools/list", {}, f"{prefix}-tools")
    tools = listed.get("result", {}).get("tools", [])
    expected = {
        "list_knowledge_bases",
        "search_knowledge",
        "get_source_excerpt",
        "ask_knowledge_agent",
        "list_publications",
        "list_subscriptions",
    }
    if status != 200 or {item.get("name") for item in tools} != expected:
        raise RuntimeError("MCP tool discovery did not return the six-tool read-only surface")
    if not all(item.get("annotations", {}).get("readOnlyHint") is True for item in tools):
        raise RuntimeError("MCP tool annotations are not read-only")

    library = tool_call(client, bob_token, "list_knowledge_bases", {}, f"{prefix}-library")
    listed_ids = {str(item.get("id")) for item in library.get("items", [])}
    if kb_ids["shared"] not in listed_ids or kb_ids["public"] in listed_ids or kb_ids["private"] in listed_ids:
        raise RuntimeError("MCP default library scope is incorrect")
    search = tool_call(
        client,
        bob_token,
        "search_knowledge",
        {"query": sentinels["shared"], "knowledge_base_ids": [kb_ids["shared"]]},
        f"{prefix}-search",
    )
    results = search.get("results", [])
    if not results or any(str(item.get("knowledge_base_id")) != kb_ids["shared"] for item in results):
        raise RuntimeError("MCP search was empty or escaped scope")
    excerpt = tool_call(
        client,
        bob_token,
        "get_source_excerpt",
        {"chunk_id": str(results[0]["id"]), "max_chars": 500},
        f"{prefix}-excerpt",
    )
    if str(excerpt.get("knowledge_base_id")) != kb_ids["shared"] or len(str(excerpt.get("content", ""))) > 500:
        raise RuntimeError("MCP excerpt contract failed")
    answer = tool_call(
        client,
        bob_token,
        "ask_knowledge_agent",
        {"query": sentinels["shared"], "knowledge_base_ids": [kb_ids["shared"]]},
        f"{prefix}-ask",
    )
    if not answer.get("answer") or any(str(item.get("knowledge_base_id")) != kb_ids["shared"] for item in answer.get("references", [])):
        raise RuntimeError("MCP grounded answer contract failed")
    publications = tool_call(client, bob_token, "list_publications", {"tag": "phase4"}, f"{prefix}-publications")
    if not any(item.get("publication", {}).get("knowledge_base_id") == kb_ids["public"] for item in publications.get("items", [])):
        raise RuntimeError("MCP publication list omitted visible public knowledge")
    tool_call(client, bob_token, "list_subscriptions", {}, f"{prefix}-subscriptions")

    status, denied = rpc(
        client,
        bob_token,
        "tools/call",
        {"name": "search_knowledge", "arguments": {"query": "private", "knowledge_base_ids": [kb_ids["private"]]}},
        f"{prefix}-denied",
        name="search_knowledge",
    )
    if status != 200:
        raise RuntimeError("MCP scope denial did not use JSON-RPC")
    expect_rpc_error(denied, -32004)

    grant = fixture["grant"]
    client.request(
        "DELETE",
        f"/api/v1/mindcreek/knowledge-bases/{kb_ids['shared']}/grants/{grant['id']}",
        {"expected_revision": int(grant["revision"])},
        token=alice_token,
    )
    _, revoked = rpc(
        client,
        bob_token,
        "tools/call",
        {"name": "get_source_excerpt", "arguments": {"chunk_id": str(results[0]["id"])}},
        f"{prefix}-revoked",
        name="get_source_excerpt",
    )
    expect_rpc_error(revoked, -32004)

    legacy_status, legacy = rpc(
        client,
        bob_token,
        "initialize",
        {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "phase4-probe", "version": "1"}},
        f"{prefix}-legacy",
        modern=False,
    )
    if legacy_status != 200 or legacy.get("result", {}).get("protocolVersion") != "2025-11-25":
        raise RuntimeError("legacy MCP compatibility initialization failed")

    anonymous = urllib.request.Request(
        client.base_url + "/mcp",
        data=b'{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}',
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        urllib.request.urlopen(anonymous, timeout=30)
        raise RuntimeError("anonymous MCP request was accepted")
    except urllib.error.HTTPError as error:
        if error.code != 401:
            raise
    wrong_status, _ = rpc(
        client,
        bob_token,
        "ping",
        {},
        f"{prefix}-workspace",
        extra_headers={"X-Tenant-ID": "999999"},
    )
    if wrong_status != 403:
        raise RuntimeError("cross-workspace MCP request was accepted")

    events = audit_count(prefix)
    if events < 8:
        raise RuntimeError(f"MCP audit count is too small: {events}")
    report = {
        "modern_protocol": MODERN_PROTOCOL,
        "legacy_protocol": "2025-11-25",
        "tools": sorted(expected),
        "anonymous": "denied",
        "wrong_workspace": "denied",
        "scope_denial": "non-disclosing",
        "revocation": "immediate",
        "audit_events": events,
    }
    path = Path(args.report)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Phase 4 Gate C passed: authenticated hosted MCP, six tools, scope, revocation, and audit")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Phase 4 Gate C probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
