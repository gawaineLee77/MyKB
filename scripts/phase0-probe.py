#!/usr/bin/env python3
"""Run the Phase 0 two-user authorization and retrieval baseline probe."""

from __future__ import annotations

import argparse
import json
import secrets
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
PRIVATE_NOTE = ROOT / "testdata/phase0/alice-private.md"
PUBLIC_NOTE_EN = ROOT / "testdata/phase0/public-handbook-en.md"
PUBLIC_NOTE_ZH = ROOT / "testdata/phase0/public-handbook-zh.md"
PRIVATE_SENTINEL = "MYKB_ALICE_PRIVATE_7F3A"
PUBLIC_SENTINEL_EN = "MYKB_PUBLIC_RUNBOOK_42B9"
PUBLIC_SENTINEL_ZH = "MYKB_PUBLIC_ZH_91C2"


class APIError(RuntimeError):
    def __init__(self, status: int, method: str, path: str, body: Any):
        super().__init__(f"{method} {path} returned HTTP {status}: {body}")
        self.status = status
        self.body = body


class Client:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip("/")

    def request(
        self,
        method: str,
        path: str,
        payload: Any | None = None,
        token: str | None = None,
        allowed: tuple[int, ...] = (200,),
    ) -> tuple[int, Any]:
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        headers = {"Accept": "application/json"}
        if payload is not None:
            headers["Content-Type"] = "application/json"
        if token:
            headers["Authorization"] = f"Bearer {token}"
        request = urllib.request.Request(
            f"{self.base_url}{path}", data=data, headers=headers, method=method
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                status = response.status
                raw = response.read()
        except urllib.error.HTTPError as exc:
            status = exc.code
            raw = exc.read()
        try:
            body = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            body = raw.decode("utf-8", errors="replace")
        if status not in allowed:
            raise APIError(status, method, path, body)
        return status, body


def wait_for_health(base_url: str, timeout_seconds: int) -> None:
    deadline = time.time() + timeout_seconds
    health_url = f"{base_url.rstrip('/')}/health"
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(health_url, timeout=3) as response:
                if response.status == 200:
                    return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(2)
    raise RuntimeError(f"WeKnora did not become healthy: {health_url}")


def login(client: Client, email: str, password: str) -> dict[str, Any]:
    _, body = client.request(
        "POST", "/api/v1/auth/login", {"email": email, "password": password}
    )
    if not body.get("token") or not body.get("active_tenant"):
        raise RuntimeError(f"login response is missing token or active_tenant: {body}")
    return body


def wait_for_knowledge(client: Client, token: str, knowledge_id: str) -> dict[str, Any]:
    deadline = time.time() + 120
    last: dict[str, Any] = {}
    while time.time() < deadline:
        _, body = client.request("GET", f"/api/v1/knowledge/{knowledge_id}", token=token)
        last = body.get("data", {})
        status = last.get("parse_status")
        if status == "completed":
            return last
        if status == "failed":
            raise RuntimeError(f"manual note processing failed: {last.get('error_message', '')}")
        time.sleep(2)
    raise RuntimeError(f"manual note did not finish processing: {last}")


def create_manual_note(
    client: Client, token: str, kb_id: str, title: str, path: Path
) -> str:
    _, body = client.request(
        "POST",
        f"/api/v1/knowledge-bases/{kb_id}/knowledge/manual",
        {
            "title": title,
            "content": path.read_text(encoding="utf-8"),
            "status": "publish",
            "channel": "phase0",
        },
        token=token,
    )
    knowledge_id = body["data"]["id"]
    wait_for_knowledge(client, token, knowledge_id)
    return knowledge_id


def search_contains(
    client: Client, token: str, kb_id: str, sentinel: str
) -> tuple[bool, int]:
    _, body = client.request(
        "POST",
        f"/api/v1/knowledge-bases/{kb_id}/hybrid-search",
        {
            "query_text": sentinel,
            "vector_threshold": 0,
            "keyword_threshold": 0,
            "match_count": 5,
        },
        token=token,
    )
    results = body.get("data", [])
    return sentinel in json.dumps(results, ensure_ascii=False), len(results)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:18081")
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument(
        "--report", default=str(ROOT / ".local/phase0-probe-report.json")
    )
    args = parser.parse_args()

    wait_for_health(args.base_url, args.health_timeout)
    client = Client(args.base_url)
    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    alice_email = f"phase0-alice-{nonce}@example.invalid"
    bob_email = f"phase0-bob-{nonce}@example.invalid"
    alice_password = f"Phase0-A-{secrets.token_urlsafe(12)}!"
    bob_password = f"Phase0-B-{secrets.token_urlsafe(12)}!"

    for username, email, password in (
        (f"phase0-alice-{nonce}", alice_email, alice_password),
        (f"phase0-bob-{nonce}", bob_email, bob_password),
    ):
        client.request(
            "POST",
            "/api/v1/auth/register",
            {"username": username, "email": email, "password": password},
            allowed=(201,),
        )

    alice = login(client, alice_email, alice_password)
    bob = login(client, bob_email, bob_password)
    alice_token = alice["token"]
    bob_token = bob["token"]
    alice_tenant_id = int(alice["active_tenant"]["id"])

    _, model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": "mykb-phase0-embedding",
            "type": "Embedding",
            "source": "remote",
            "description": "Deterministic synthetic Phase 0 model",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "phase0-not-a-secret",
                "provider": "generic",
                "embedding_parameters": {"dimension": 64, "truncate_prompt_tokens": 0},
            },
        },
        token=alice_token,
        allowed=(201,),
    )
    model_id = model_body["data"]["id"]

    _, kb_body = client.request(
        "POST",
        "/api/v1/knowledge-bases",
        {
            "name": f"phase0-alice-private-{nonce}",
            "description": "Synthetic owner-private note space",
            "type": "document",
            "embedding_model_id": model_id,
            "storage_provider_config": {"provider": "local"},
            "question_generation_config": {"enabled": False, "question_count": 0},
        },
        token=alice_token,
        allowed=(201,),
    )
    kb_id = kb_body["data"]["id"]

    cross_status, _ = client.request(
        "GET",
        f"/api/v1/knowledge-bases/{kb_id}",
        token=bob_token,
        allowed=(403, 404),
    )

    create_manual_note(
        client,
        alice_token,
        kb_id,
        "Alice private synthetic note",
        PRIVATE_NOTE,
    )

    _, public_kb_body = client.request(
        "POST",
        "/api/v1/knowledge-bases",
        {
            "name": f"phase0-public-rag-{nonce}",
            "description": "Synthetic bilingual Plain RAG corpus",
            "type": "document",
            "embedding_model_id": model_id,
            "storage_provider_config": {"provider": "local"},
            "question_generation_config": {"enabled": False, "question_count": 0},
        },
        token=alice_token,
        allowed=(201,),
    )
    public_kb_id = public_kb_body["data"]["id"]
    create_manual_note(
        client, alice_token, public_kb_id, "Shared service handbook", PUBLIC_NOTE_EN
    )
    create_manual_note(
        client, alice_token, public_kb_id, "共享服务手册", PUBLIC_NOTE_ZH
    )
    english_retrieved, _ = search_contains(
        client, alice_token, public_kb_id, PUBLIC_SENTINEL_EN
    )
    chinese_retrieved, _ = search_contains(
        client, alice_token, public_kb_id, PUBLIC_SENTINEL_ZH
    )
    if not english_retrieved or not chinese_retrieved:
        raise RuntimeError("bilingual Plain RAG retrieval did not return both sentinels")

    _, invitation_body = client.request(
        "POST",
        f"/api/v1/tenants/{alice_tenant_id}/invitations",
        {"email": bob_email, "role": "viewer", "message": "Phase 0 scope probe"},
        token=alice_token,
        allowed=(201,),
    )
    invitation_id = int(invitation_body["data"]["id"])
    client.request(
        "POST",
        f"/api/v1/me/invitations/{invitation_id}/accept",
        token=bob_token,
    )
    _, switched = client.request(
        "POST",
        "/api/v1/auth/switch-tenant",
        {"tenant_id": alice_tenant_id, "refresh_token": bob.get("refresh_token", "")},
        token=bob_token,
    )
    bob_shared_token = switched["token"]

    _, list_body = client.request(
        "GET", "/api/v1/knowledge-bases", token=bob_shared_token
    )
    visible_ids = {item["id"] for item in list_body.get("data", [])}
    same_workspace_list_visible = kb_id in visible_ids

    same_workspace_detail_status, _ = client.request(
        "GET",
        f"/api/v1/knowledge-bases/{kb_id}",
        token=bob_shared_token,
        allowed=(200,),
    )
    sentinel_retrieved, retrieval_result_count = search_contains(
        client, bob_shared_token, kb_id, PRIVATE_SENTINEL
    )

    if not same_workspace_list_visible or not sentinel_retrieved:
        raise RuntimeError(
            "expected upstream same-workspace visibility/retrieval gap was not reproduced"
        )

    report = {
        "baseline": "WeKnora v0.7.2",
        "synthetic_run": nonce,
        "cross_workspace_detail_status": cross_status,
        "same_workspace_list_exposes_owner_kb": same_workspace_list_visible,
        "same_workspace_detail_status": same_workspace_detail_status,
        "same_workspace_retrieval_exposes_owner_note": sentinel_retrieved,
        "private_retrieval_result_count": retrieval_result_count,
        "plain_rag_english_retrieved": english_retrieved,
        "plain_rag_chinese_retrieved": chinese_retrieved,
        "conclusion": "Product Gateway owner-only authorization is required for Personal Notes.",
    }
    report_path = Path(args.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError) as exc:
        print(f"phase0 probe failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
