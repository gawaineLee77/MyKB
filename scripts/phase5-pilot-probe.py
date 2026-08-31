#!/usr/bin/env python3
"""Measure zero-key multilingual pilot retrieval and grounded Ask behavior."""

from __future__ import annotations

import argparse
import json
import os
import runpy
import secrets
import statistics
import sys
import time
import urllib.error
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
RAG = runpy.run_path(str(ROOT / "scripts/phase1-gate-d-probe.py"))
Client, APIError = RAG["Client"], RAG["APIError"]
login, multipart_request = RAG["login"], RAG["multipart_request"]
wait_for_documents, wait_for_gateway = RAG["wait_for_documents"], RAG["wait_for_gateway"]
normal_chat, verify_openable_citations = RAG["normal_chat"], RAG["verify_openable_citations"]
CHAT_ID = "builtin-mindcreek-chat"


def percentile(values: list[float], fraction: float) -> float:
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, int((len(ordered) - 1) * fraction))]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--dataset", type=Path, default=ROOT / "testdata/phase5/pilot-benchmark.json")
    parser.add_argument("--report", type=Path, default=ROOT / ".local/phase5-pilot-report.json")
    args = parser.parse_args()
    dataset = json.loads(args.dataset.read_text(encoding="utf-8"))
    client = Client(args.base_url)
    wait_for_gateway(client, 240)
    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    email = f"phase5-pilot-{nonce}@example.invalid"
    password = f"Phase5-{secrets.token_urlsafe(12)}!"
    client.request("POST", "/api/v1/auth/register", {"username": f"phase5-pilot-{nonce}", "email": email, "password": password}, allowed=(201,))
    token = str(login(client, email, password)["token"])

    _, models = client.request("GET", "/api/v1/mindcreek/models", token=token)
    if not models.get("data", {}).get("ready"):
        raise RuntimeError("managed models are not ready for the pilot")
    _, created = client.request(
        "POST", "/api/v1/knowledge-spaces",
        {"mode": "rag", "index_profile": "plain", "name": f"Phase 5 Pilot {nonce}", "storage_provider": "local"},
        token=token, allowed=(201,), headers={"Idempotency-Key": f"phase5-pilot-{nonce}"},
    )
    kb_id = str(created["data"]["knowledge_base_id"])
    ingestion = f"/api/v1/knowledge-bases/{kb_id}/ingestions"
    ids: set[str] = set()
    for document in dataset["documents"]:
        _, body = multipart_request(client, ingestion, document["file"], document["content"].encode("utf-8"), token, (202,))
        ids.add(str(body["data"]["id"]))
    terminal = wait_for_documents(client, ingestion, token, ids)
    if any(terminal[item].get("parse_status") != "completed" for item in ids):
        raise RuntimeError("pilot documents did not finish ingestion")

    reciprocal_ranks: list[float] = []
    latencies: list[float] = []
    language_hits: dict[str, int] = {"en": 0, "zh": 0}
    for query in dataset["queries"]:
        started = time.perf_counter()
        _, body = client.request(
            "POST", f"/api/v1/knowledge-bases/{kb_id}/hybrid-search",
            {"query_text": query["query"], "vector_threshold": 0, "keyword_threshold": 0, "match_count": 5}, token=token,
        )
        latencies.append((time.perf_counter() - started) * 1000)
        results = list(body.get("data", []))
        rank = next((index for index, item in enumerate(results, 1) if query["expected"] in json.dumps(item, ensure_ascii=False)), 0)
        reciprocal_ranks.append(1 / rank if rank else 0)
        language_hits[query["language"]] += int(rank > 0)

    recall = sum(value > 0 for value in reciprocal_ranks) / len(reciprocal_ranks)
    mrr = statistics.mean(reciprocal_ranks)
    if recall < 1.0 or mrr < 0.75:
        raise RuntimeError(f"pilot retrieval target missed: recall@5={recall:.2f}, MRR={mrr:.2f}")
    citations = 0
    for query in (dataset["queries"][0], dataset["queries"][2]):
        _, session = client.request("POST", "/api/v1/sessions", {"title": "Phase 5 pilot groundedness"}, token=token, allowed=(201,))
        events = normal_chat(client, token, str(session["data"]["id"]), kb_id, CHAT_ID, query["query"])
        citations += verify_openable_citations(client, token, kb_id, events, query["expected"])

    report: dict[str, Any] = {
        "status": "pass", "dataset": "synthetic bilingual internal-handbook fixture",
        "ordinary_user_keys_entered": 0, "documents": len(ids), "queries": len(reciprocal_ranks),
        "recall_at_5": recall, "mean_reciprocal_rank": round(mrr, 4), "language_hits": language_hits,
        "latency_ms": {"p50": round(statistics.median(latencies), 2), "p95": round(percentile(latencies, 0.95), 2)},
        "grounded_ask_languages": 2, "openable_citations": citations,
        "provider_cost": "synthetic provider; production billing review required", "private_documents_used": False,
    }
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    args.report.chmod(0o600)
    print(f"Phase 5 pilot baseline passed: Recall@5={recall:.2f}, MRR={mrr:.2f}, bilingual grounded Ask")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, OSError, RuntimeError, ValueError, urllib.error.URLError) as error:
        print(f"Phase 5 pilot probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
