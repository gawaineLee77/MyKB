#!/usr/bin/env python3
"""Verify Phase 5 zero-key managed-model onboarding with synthetic data."""

from __future__ import annotations

import argparse
import json
import os
import runpy
import secrets
import subprocess
import sys
import time
import urllib.error
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
RAG = runpy.run_path(str(ROOT / "scripts/phase1-gate-d-probe.py"))
WEB_AGENT = runpy.run_path(str(ROOT / "scripts/phase4-gate-b-probe.py"))
Client = RAG["Client"]
APIError = RAG["APIError"]
login = RAG["login"]
multipart_request = RAG["multipart_request"]
wait_for_documents = RAG["wait_for_documents"]
wait_for_gateway = RAG["wait_for_gateway"]
hybrid_search = RAG["hybrid_search"]
normal_chat = RAG["normal_chat"]
verify_openable_citations = RAG["verify_openable_citations"]
stream_chat = WEB_AGENT["stream_chat"]

MANAGED = {
    "KnowledgeQA": "builtin-mindcreek-chat",
    "Embedding": "builtin-mindcreek-embedding",
    "Rerank": "builtin-mindcreek-rerank",
}


def assert_redacted(payload: Any) -> None:
    serialized = json.dumps(payload, ensure_ascii=False).lower()
    for forbidden in ("api_key", "base_url", "parameters", "credential", "development-only", "mock-embedding"):
        if forbidden in serialized:
            raise RuntimeError(f"safe model response disclosed forbidden field {forbidden!r}")


def assert_default_snapshot(body: dict[str, Any]) -> None:
    data = body.get("data", {})
    defaults = list(data.get("defaults", []))
    by_type = {str(model.get("type")): model for model in defaults}
    if not data.get("ready") or data.get("overrides_enabled") or len(defaults) != 3:
        raise RuntimeError("managed model snapshot is not ready or securely defaulted")
    for model_type, expected_id in MANAGED.items():
        model = by_type.get(model_type, {})
        if model.get("id") != expected_id or not model.get("managed") or not model.get("default") or not model.get("available"):
            raise RuntimeError(f"managed {model_type} contract is invalid")
    assert_redacted(body)


def assert_logs_redacted() -> None:
    result = subprocess.run(
        ["docker", "logs", "MindCreek-app", "--since", "15m"],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError("unable to inspect application logs for credential redaction")
    if "development-only" in result.stdout:
        raise RuntimeError("managed development credential appeared in application logs")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=240)
    parser.add_argument("--report", default=str(ROOT / ".local/phase5-gate-a-report.json"))
    args = parser.parse_args()
    client = Client(args.base_url)
    wait_for_gateway(client, args.health_timeout)

    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    email = f"phase5-zero-key-{nonce}@example.invalid"
    password = f"Phase5-{secrets.token_urlsafe(12)}!"
    client.request(
        "POST",
        "/api/v1/auth/register",
        {"username": f"phase5-zero-key-{nonce}", "email": email, "password": password},
        allowed=(201,),
    )
    token = str(login(client, email, password)["token"])

    _, model_body = client.request("GET", "/api/v1/mindcreek/models", token=token)
    assert_default_snapshot(model_body)

    disabled_status, disabled_body = client.request(
        "POST",
        "/api/v1/mindcreek/models/overrides",
        {"name": "disabled", "display_name": "disabled", "type": "KnowledgeQA", "provider": "generic", "base_url": "https://models.example.invalid/v1", "api_key": "synthetic"},
        token=token,
        headers={"X-Request-ID": f"phase5-disabled-{nonce}"},
        allowed=(404,),
    )
    if disabled_status != 404 or disabled_body.get("error", {}).get("code") != "models.overrides_disabled":
        raise RuntimeError("disabled override endpoint did not return its non-disclosing response")
    raw_status, raw_body = client.request(
        "POST",
        "/api/v1/models",
        {"name": "bypass", "type": "KnowledgeQA", "source": "remote", "parameters": {}},
        token=token,
        allowed=(404,),
    )
    if raw_status != 404 or raw_body.get("error", {}).get("code") != "models.raw_route_disabled":
        raise RuntimeError("raw upstream model mutation bypass is open")

    _, space_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        {"mode": "rag", "index_profile": "plain", "name": f"Phase 5 Zero Key {nonce}", "description": "Synthetic managed-model acceptance", "storage_provider": "local"},
        token=token,
        allowed=(201,),
        headers={"Idempotency-Key": f"phase5-zero-key-{nonce}"},
    )
    kb_id = str(space_body["data"]["knowledge_base_id"])
    _, kb_body = client.request("GET", f"/api/v1/knowledge-bases/{kb_id}", token=token)
    kb = kb_body.get("data", {})
    if kb.get("embedding_model_id") != MANAGED["Embedding"] or kb.get("summary_model_id") != MANAGED["KnowledgeQA"]:
        raise RuntimeError("knowledge space did not receive managed embedding and chat defaults")
    _, profile_body = client.request("GET", f"/api/v1/knowledge-bases/{kb_id}/product-profile", token=token)
    effective = profile_body.get("data", {}).get("effective_config", {})
    if isinstance(effective, str):
        effective = json.loads(effective)
    retrieval = effective.get("retrieval", {})
    if not retrieval.get("rerank_enabled") or retrieval.get("rerank_model_id") != MANAGED["Rerank"]:
        raise RuntimeError("product profile did not record the managed reranker")

    sentinel = f"MINDCREEK_PHASE5_{nonce}"
    ingestion_path = f"/api/v1/knowledge-bases/{kb_id}/ingestions"
    _, upload = multipart_request(
        client,
        ingestion_path,
        "managed-models.md",
        f"# Managed models\n\nEnglish marker {sentinel}.\n\n中文知识标记 {sentinel}。".encode("utf-8"),
        token,
        (202,),
    )
    document_id = str(upload["data"]["id"])
    documents = wait_for_documents(client, ingestion_path, token, {document_id})
    if documents[document_id].get("parse_status") != "completed":
        raise RuntimeError("zero-key document ingestion failed")

    hits = hybrid_search(client, kb_id, token, sentinel, sentinel)
    _, session_body = client.request("POST", "/api/v1/sessions", {"title": "Phase 5 zero-key Ask"}, token=token, allowed=(201,))
    session_id = str(session_body["data"]["id"])
    events = normal_chat(client, token, session_id, kb_id, MANAGED["KnowledgeQA"], sentinel)
    citations = verify_openable_citations(client, token, kb_id, events, sentinel)

    _, agent_body = client.request("GET", "/api/v1/agents/builtin-smart-reasoning", token=token)
    agent_config = agent_body.get("data", {}).get("config", {})
    if agent_config.get("model_id") != MANAGED["KnowledgeQA"] or agent_config.get("rerank_model_id") != MANAGED["Rerank"] or agent_config.get("web_search_enabled"):
        raise RuntimeError("Smart Reasoning is not bound to the managed chat/rerank pair")
    reasoning = stream_chat(
        client,
        token,
        session_id,
        [kb_id],
        "Summarize the managed-model marker with evidence",
        f"phase5-reasoning-{nonce}",
        reasoning=True,
        summary_model_id=MANAGED["KnowledgeQA"],
    )
    assert_logs_redacted()

    report = {
        "managed_snapshot": "redacted and ready",
        "ordinary_user_keys_entered": 0,
        "automatic_defaults": sorted(MANAGED.values()),
        "override_capability": "disabled and non-disclosing",
        "raw_model_mutation_bypass": "closed",
        "ingestion": "pass",
        "hybrid_retrieval_hits": len(hits),
        "normal_ask": "pass",
        "openable_citations": citations,
        "smart_reasoning_events": len(reasoning),
        "application_log_secret_scan": "pass",
        "kb_id": kb_id,
    }
    report_path = Path(args.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Phase 5 Gate A passed: zero-key create, ingest, rerank, Ask, reasoning, and redaction")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError, ValueError) as error:
        print(f"Phase 5 Gate A probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
