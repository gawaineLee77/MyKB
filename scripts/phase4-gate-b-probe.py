#!/usr/bin/env python3
"""Run the live Phase 4 authorized Web-agent and retrieval baseline matrix."""

from __future__ import annotations

import argparse
import json
import os
import runpy
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
BASE = runpy.run_path(str(ROOT / "scripts/phase1-gate-b-probe.py"))
SUPPORT = runpy.run_path(str(ROOT / "scripts/phase4_probe_support.py"))
Client = SUPPORT["Client"]
APIError = SUPPORT["APIError"]
expect_error = BASE["expect_error"]
wait_for_gateway = SUPPORT["wait_for_gateway"]
create_fixture = SUPPORT["create_fixture"]


def stream_chat(
    client: Any,
    token: str,
    session_id: str,
    kb_ids: list[str],
    query: str,
    correlation: str,
    *,
    reasoning: bool = False,
    summary_model_id: str = "",
) -> list[dict[str, Any]]:
    endpoint = "agent-chat" if reasoning else "knowledge-chat"
    payload: dict[str, Any] = {
        "query": query,
        "knowledge_base_ids": kb_ids,
        "agent_enabled": reasoning,
        "web_search_enabled": True,
        "mcp_service_ids": ["must-be-removed"],
        "disable_title": True,
        "channel": "web",
    }
    if reasoning:
        payload["agent_id"] = "builtin-smart-reasoning"
        payload["summary_model_id"] = summary_model_id
    request = urllib.request.Request(
        client.base_url + f"/api/v1/{endpoint}/{session_id}",
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Accept": "text/event-stream",
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "X-Request-ID": correlation,
        },
        method="POST",
    )
    events: list[dict[str, Any]] = []
    total = 0
    with urllib.request.urlopen(request, timeout=120) as response:
        if response.status != 200:
            raise RuntimeError(f"{endpoint} did not open an SSE stream")
        for raw_line in response:
            total += len(raw_line)
            if total > 4 * 1024 * 1024:
                raise RuntimeError(f"{endpoint} returned an oversized stream")
            line = raw_line.decode("utf-8").strip()
            if not line.startswith("data:"):
                continue
            value = line[5:].strip()
            if not value or value == "[DONE]":
                continue
            event = json.loads(value)
            events.append(event)
            if event.get("response_type") == "error":
                raise RuntimeError(f"{endpoint} stream failed")
    if not any((event.get("response_type") or event.get("type")) == "answer" for event in events):
        raise RuntimeError(f"{endpoint} returned no answer")
    return events


def references(events: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [reference for event in events for reference in event.get("knowledge_references", [])]


def load_env() -> dict[str, str]:
    values: dict[str, str] = {}
    for raw in (ROOT / ".local/mindcreek.env").read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def audit_count(correlations: list[str]) -> int:
    values = load_env()
    quoted = ",".join("'" + value.replace("'", "''") + "'" for value in correlations)
    sql = (
        "SELECT count(*) FROM mindcreek.agent_operation_audit_events "
        f"WHERE correlation_id IN ({quoted})"
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
        raise RuntimeError("unable to verify Phase 4 agent audit events")
    return int(result.stdout.strip())


def search(client: Any, token: str, query: str, kb_ids: list[str] | None, correlation: str) -> tuple[list[dict[str, Any]], float]:
    payload: dict[str, Any] = {"query": query}
    if kb_ids is not None:
        payload["knowledge_base_ids"] = kb_ids
    started = time.perf_counter()
    _, body = client.request(
        "POST",
        "/api/v1/knowledge-search",
        payload,
        token=token,
        headers={"X-Request-ID": correlation},
    )
    return list(body.get("data", [])), (time.perf_counter() - started) * 1000


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--report", default=str(ROOT / ".local/phase4-gate-b-report.json"))
    args = parser.parse_args()
    client = Client(args.base_url)
    wait_for_gateway(client, args.health_timeout)
    fixture = create_fixture(client, "phase4-web")
    alice_token, bob_token = fixture["alice_token"], fixture["bob_token"]
    kb_ids, sentinels = fixture["kb_ids"], fixture["sentinels"]
    correlations = [f"phase4-{name}-{fixture['nonce']}" for name in ("default", "explicit", "quick", "reasoning")]

    _, default_scope = client.request("GET", "/api/v1/mindcreek/agent/scope", token=bob_token)
    default_ids = set(default_scope["data"]["knowledge_base_ids"])
    if kb_ids["shared"] not in default_ids or kb_ids["public"] in default_ids or kb_ids["private"] in default_ids:
        raise RuntimeError(f"default scope is incorrect: {sorted(default_ids)}")
    _, explicit_scope = client.request(
        "POST",
        "/api/v1/mindcreek/agent/scope/resolve",
        {"selection": "explicit", "knowledge_base_ids": [kb_ids["public"], kb_ids["shared"]]},
        token=bob_token,
    )
    if set(explicit_scope["data"]["knowledge_base_ids"]) != {kb_ids["public"], kb_ids["shared"]}:
        raise RuntimeError("explicit public selection was not preserved")
    expect_error(
        client,
        "POST",
        "/api/v1/mindcreek/agent/scope/resolve",
        404,
        "resource.not_found",
        payload={"selection": "explicit", "knowledge_base_ids": [kb_ids["private"]]},
        token=bob_token,
    )

    latencies: list[float] = []
    english, latency = search(client, bob_token, sentinels["shared"], None, correlations[0])
    latencies.append(latency)
    chinese, latency = search(client, bob_token, "中文检索标记", [kb_ids["shared"]], correlations[1])
    latencies.append(latency)
    for label, results, expected in (("English", english, sentinels["shared"]), ("Chinese", chinese, "中文检索标记")):
        if not any(expected in str(item.get("content", "")) for item in results[:5]):
            raise RuntimeError(f"{label} Recall@5 baseline failed")
        if any(str(item.get("knowledge_base_id")) != kb_ids["shared"] for item in results):
            raise RuntimeError(f"{label} search escaped its effective scope")

    _, session = client.request("POST", "/api/v1/sessions", {"title": "Phase 4 authorized Ask"}, token=bob_token, allowed=(201,))
    session_id = str(session["data"]["id"])
    quick_events = stream_chat(client, bob_token, session_id, [kb_ids["shared"]], sentinels["shared"], correlations[2])
    quick_references = references(quick_events)
    if not quick_references:
        raise RuntimeError("quick RAG returned no grounded reference")
    citation = quick_references[0]
    if str(citation.get("knowledge_base_id")) != kb_ids["shared"]:
        raise RuntimeError("quick RAG returned an out-of-scope reference")
    chunk_id = str(citation.get("id", ""))
    client.request("GET", f"/api/v1/chunks/by-id/{chunk_id}", token=bob_token)

    stream_chat(
        client,
        bob_token,
        session_id,
        [kb_ids["shared"]],
        "Summarize with evidence",
        correlations[3],
        reasoning=True,
        summary_model_id=fixture["chat_model_id"],
    )

    grant = fixture["grant"]
    client.request(
        "DELETE",
        f"/api/v1/mindcreek/knowledge-bases/{kb_ids['shared']}/grants/{grant['id']}",
        {"expected_revision": int(grant["revision"])},
        token=alice_token,
        headers={"X-Request-ID": f"phase4-revoke-{fixture['nonce']}"},
    )
    expect_error(
        client,
        "POST",
        "/api/v1/knowledge-search",
        404,
        "resource.not_found",
        payload={"query": sentinels["shared"], "knowledge_base_ids": [kb_ids["shared"]]},
        token=bob_token,
    )
    expect_error(client, "GET", f"/api/v1/chunks/by-id/{chunk_id}", 404, "resource.not_found", token=bob_token)
    expect_error(client, "GET", f"/api/v1/messages/{session_id}/load", 404, "resource.not_found", token=bob_token)

    publication = fixture["publication"]
    client.request(
        "DELETE",
        f"/api/v1/mindcreek/knowledge-bases/{kb_ids['public']}/publication",
        {"expected_row_version": int(publication["row_version"])},
        token=alice_token,
        headers={"X-Request-ID": f"phase4-unpublish-{fixture['nonce']}"},
    )
    expect_error(
        client,
        "POST",
        "/api/v1/mindcreek/agent/scope/resolve",
        404,
        "resource.not_found",
        payload={"selection": "explicit", "knowledge_base_ids": [kb_ids["public"]]},
        token=bob_token,
    )

    events = audit_count(correlations)
    if events < len(correlations):
        raise RuntimeError(f"agent audit count is too small: {events}")
    report = {
        "default_scope": "owned/shared/subscribed only",
        "organization_public": "explicit only",
        "private_scope": "non-disclosing denial",
        "quick_rag": "grounded",
        "reasoning": "pass",
        "revocation": "immediate",
        "unpublish": "immediate",
        "retrieval": {"queries": 2, "recall_at_5": 1.0, "latency_ms": latencies, "max_latency_ms": max(latencies)},
        "agent_audit_events": events,
    }
    path = Path(args.report)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Phase 4 Gate B passed: Web scope, grounded sources, revocation, and multilingual baseline")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Phase 4 Gate B probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
