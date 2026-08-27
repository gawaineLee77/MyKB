#!/usr/bin/env python3
"""Run the compact two-user Personal Notes authorization matrix."""

from __future__ import annotations

import argparse
import json
import os
import secrets
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
NOTE_FILE = ROOT / "testdata/phase0/alice-private.md"
SENTINEL = "MYKB_ALICE_PRIVATE_7F3A"


class APIError(RuntimeError):
    pass


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
        headers: dict[str, str] | None = None,
    ) -> tuple[int, Any]:
        data = None if payload is None else json.dumps(payload).encode("utf-8")
        request_headers = {"Accept": "application/json"}
        if payload is not None:
            request_headers["Content-Type"] = "application/json"
        if token:
            request_headers["Authorization"] = f"Bearer {token}"
        if headers:
            request_headers.update(headers)
        request = urllib.request.Request(
            self.base_url + path,
            data=data,
            headers=request_headers,
            method=method,
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                status = response.status
                raw = response.read(4 * 1024 * 1024 + 1)
        except urllib.error.HTTPError as error:
            status = error.code
            raw = error.read(4 * 1024 * 1024 + 1)
        if len(raw) > 4 * 1024 * 1024:
            raise APIError(f"{method} {path} returned an oversized response")
        try:
            body = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            body = raw.decode("utf-8", errors="replace")
        if status not in allowed:
            code = error_code(body)
            raise APIError(f"{method} {path} returned HTTP {status} ({code})")
        return status, body


def error_code(body: Any) -> str:
    if not isinstance(body, dict):
        return "non-json"
    error = body.get("error", {})
    return error.get("code", "unknown") if isinstance(error, dict) else "unknown"


def wait_for_gateway(client: Client, timeout_seconds: int) -> None:
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        try:
            status, _ = client.request("GET", "/api/v1/auth/config")
            if status == 200:
                return
        except (APIError, OSError, urllib.error.URLError):
            pass
        time.sleep(2)
    raise RuntimeError("MindCreek gateway did not become ready")


def login(client: Client, email: str, password: str) -> dict[str, Any]:
    _, body = client.request(
        "POST", "/api/v1/auth/login", {"email": email, "password": password}
    )
    if not body.get("token") or not body.get("active_tenant") or not body.get("user"):
        raise RuntimeError("login response is missing principal fields")
    return body


def create_manual_note(client: Client, token: str, kb_id: str) -> str:
    _, body = client.request(
        "POST",
        f"/api/v1/knowledge-bases/{kb_id}/knowledge/manual",
        {
            "title": "Gate B private synthetic note",
            "content": NOTE_FILE.read_text(encoding="utf-8"),
            "status": "publish",
            "channel": "phase1-gate-b",
        },
        token=token,
        allowed=(200,),
    )
    knowledge_id = str(body["data"]["id"])
    deadline = time.time() + 120
    while time.time() < deadline:
        _, current = client.request(
            "GET", f"/api/v1/knowledge/{knowledge_id}", token=token
        )
        parse_status = current.get("data", {}).get("parse_status")
        if parse_status == "completed":
            return knowledge_id
        if parse_status == "failed":
            raise RuntimeError("synthetic manual note processing failed")
        time.sleep(2)
    raise RuntimeError("synthetic manual note processing timed out")


def expect_error(
    client: Client,
    method: str,
    path: str,
    status: int,
    code: str,
    *,
    payload: Any | None = None,
    token: str | None = None,
    headers: dict[str, str] | None = None,
) -> None:
    actual_status, body = client.request(
        method,
        path,
        payload,
        token,
        allowed=(status,),
        headers=headers,
    )
    actual_code = error_code(body)
    if actual_status != status or actual_code != code:
        raise RuntimeError(
            f"{method} {path} returned {actual_status}/{actual_code}, "
            f"expected {status}/{code}"
        )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--base-url",
        default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"),
    )
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument(
        "--report", default=str(ROOT / ".local/phase1-gate-b-report.json")
    )
    arguments = parser.parse_args()
    client = Client(arguments.base_url)
    wait_for_gateway(client, arguments.health_timeout)

    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    alice_email = f"gate-b-alice-{nonce}@example.invalid"
    bob_email = f"gate-b-bob-{nonce}@example.invalid"
    alice_password = f"GateB-A-{secrets.token_urlsafe(12)}!"
    bob_password = f"GateB-B-{secrets.token_urlsafe(12)}!"
    for username, email, password in (
        (f"gate-b-alice-{nonce}", alice_email, alice_password),
        (f"gate-b-bob-{nonce}", bob_email, bob_password),
    ):
        client.request(
            "POST",
            "/api/v1/auth/register",
            {"username": username, "email": email, "password": password},
            allowed=(201,),
        )

    alice = login(client, alice_email, alice_password)
    bob = login(client, bob_email, bob_password)
    alice_token = str(alice["token"])
    bob_token = str(bob["token"])
    alice_tenant_id = int(alice["active_tenant"]["id"])
    bob_tenant_id = int(bob["active_tenant"]["id"])

    _, model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"gate-b-embedding-{nonce}",
            "type": "Embedding",
            "source": "remote",
            "description": "Deterministic synthetic Gate B model",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "gate-b-not-a-secret",
                "provider": "generic",
                "embedding_parameters": {
                    "dimension": 64,
                    "truncate_prompt_tokens": 0,
                },
            },
        },
        token=alice_token,
        allowed=(201,),
    )
    model_id = str(model_body["data"]["id"])
    note_key = f"gate-b-note-{nonce}"
    note_request = {
        "mode": "personal_notes",
        "index_profile": "notes_plain",
        "name": f"gate-b-private-note-{nonce}",
        "description": "Synthetic owner-only Personal Notes space",
        "embedding_model_id": model_id,
        "storage_provider": "local",
    }
    note_status, note_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        note_request,
        token=alice_token,
        allowed=(201,),
        headers={"Idempotency-Key": note_key},
    )
    note_kb_id = str(note_body["data"]["knowledge_base_id"])
    retry_status, retry_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        note_request,
        token=alice_token,
        allowed=(200,),
        headers={"Idempotency-Key": note_key},
    )
    if (
        note_status != 201
        or retry_status != 200
        or retry_body.get("data", {}).get("knowledge_base_id") != note_kb_id
        or retry_body.get("data", {}).get("reconciled") is not True
    ):
        raise RuntimeError("knowledge-space creation was not idempotent")

    _, rag_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        {
            "mode": "rag",
            "index_profile": "plain",
            "name": f"gate-b-plain-rag-{nonce}",
            "description": "Synthetic ordinary RAG control resource",
            "embedding_model_id": model_id,
            "storage_provider": "local",
        },
        token=alice_token,
        allowed=(201,),
        headers={"Idempotency-Key": f"gate-b-rag-{nonce}"},
    )
    rag_kb_id = str(rag_body["data"]["knowledge_base_id"])
    knowledge_id = create_manual_note(client, alice_token, note_kb_id)

    _, chunk_body = client.request(
        "GET", f"/api/v1/chunks/{knowledge_id}?page_size=10", token=alice_token
    )
    chunks = chunk_body.get("data", [])
    if not chunks:
        raise RuntimeError("the synthetic note produced no chunks")
    chunk_id = str(chunks[0]["id"])
    _, alice_session_body = client.request(
        "POST",
        "/api/v1/sessions",
        {"title": "Alice Gate B session", "description": "Synthetic"},
        token=alice_token,
        allowed=(201,),
    )
    alice_session_id = str(alice_session_body["data"]["id"])

    _, invitation_body = client.request(
        "POST",
        f"/api/v1/tenants/{alice_tenant_id}/invitations",
        {"email": bob_email, "role": "admin", "message": "Gate B admin bypass probe"},
        token=alice_token,
        allowed=(201,),
    )
    invitation_id = int(invitation_body["data"]["id"])
    client.request(
        "POST", f"/api/v1/me/invitations/{invitation_id}/accept", token=bob_token
    )
    _, switched = client.request(
        "POST",
        "/api/v1/auth/switch-tenant",
        {
            "tenant_id": alice_tenant_id,
            "refresh_token": bob.get("refresh_token", ""),
        },
        token=bob_token,
    )
    bob_shared_token = str(switched["token"])
    _, bob_session_body = client.request(
        "POST",
        "/api/v1/sessions",
        {"title": "Bob Gate B session", "description": "Synthetic"},
        token=bob_shared_token,
        allowed=(201,),
    )
    bob_session_id = str(bob_session_body["data"]["id"])

    _, owner_detail = client.request(
        "GET", f"/api/v1/knowledge-bases/{note_kb_id}", token=alice_token
    )
    if owner_detail.get("data", {}).get("id") != note_kb_id:
        raise RuntimeError("the Personal Notes owner could not read the note space")
    _, search_body = client.request(
        "POST",
        f"/api/v1/knowledge-bases/{note_kb_id}/hybrid-search",
        {
            "query_text": SENTINEL,
            "vector_threshold": 0,
            "keyword_threshold": 0,
            "match_count": 5,
        },
        token=alice_token,
    )
    if SENTINEL not in json.dumps(search_body.get("data", []), ensure_ascii=False):
        raise RuntimeError("the Personal Notes owner could not retrieve the sentinel")
    expect_error(
        client,
        "POST",
        f"/api/v1/knowledge-bases/{note_kb_id}/shares",
        403,
        "personal_notes.sharing_disabled",
        payload={"organization_id": "synthetic", "permission": "viewer"},
        token=alice_token,
    )

    _, list_body = client.request(
        "GET", "/api/v1/knowledge-bases", token=bob_shared_token
    )
    visible_ids = {str(item.get("id")) for item in list_body.get("data", [])}
    if note_kb_id in visible_ids or rag_kb_id not in visible_ids:
        raise RuntimeError("list filtering hid the RAG control or exposed Personal Notes")
    client.request(
        "GET", f"/api/v1/knowledge-bases/{rag_kb_id}", token=bob_shared_token
    )

    expect_error(
        client,
        "GET",
        f"/api/v1/knowledge-bases/{note_kb_id}",
        401,
        "auth.required",
    )
    expect_error(
        client,
        "GET",
        f"/api/v1/knowledge-bases/{note_kb_id}",
        401,
        "auth.invalid",
        token="invalid-gate-b-token",
    )
    expect_error(
        client,
        "GET",
        f"/api/v1/knowledge-bases/{note_kb_id}",
        403,
        "workspace.denied",
        token=alice_token,
        headers={"X-Tenant-ID": str(bob_tenant_id)},
    )

    matrix: list[tuple[str, str, str, Any | None]] = [
        ("kb-detail", "GET", f"/api/v1/knowledge-bases/{note_kb_id}", None),
        ("kb-update", "PUT", f"/api/v1/knowledge-bases/{note_kb_id}", {"name": "denied"}),
        ("source-list", "GET", f"/api/v1/knowledge-bases/{note_kb_id}/knowledge", None),
        ("faq-list", "GET", f"/api/v1/knowledge-bases/{note_kb_id}/faq/entries", None),
        ("faq-export", "GET", f"/api/v1/knowledge-bases/{note_kb_id}/faq/entries/export", None),
        ("tags", "GET", f"/api/v1/knowledge-bases/{note_kb_id}/tags", None),
        ("kb-file", "GET", f"/api/v1/knowledge-bases/{note_kb_id}/files?file_path=local%3A%2F%2Fguess", None),
        ("wiki", "GET", f"/api/v1/knowledgebase/{note_kb_id}/wiki/pages", None),
        ("source", "GET", f"/api/v1/knowledge/{knowledge_id}", None),
        ("source-stages", "GET", f"/api/v1/knowledge/{knowledge_id}/stages", None),
        ("preview", "GET", f"/api/v1/knowledge/{knowledge_id}/preview", None),
        ("download", "GET", f"/api/v1/knowledge/{knowledge_id}/download", None),
        ("image", "PUT", f"/api/v1/knowledge/image/{knowledge_id}/{chunk_id}", {"image_info": {}}),
        ("chunks", "GET", f"/api/v1/chunks/{knowledge_id}", None),
        ("chunk", "GET", f"/api/v1/chunks/by-id/{chunk_id}", None),
        ("batch-source", "GET", f"/api/v1/knowledge/batch?ids={urllib.parse.quote(knowledge_id)}", None),
        ("hybrid-search", "POST", f"/api/v1/knowledge-bases/{note_kb_id}/hybrid-search", {"query_text": SENTINEL}),
        ("knowledge-search", "POST", "/api/v1/knowledge-search", {"query": SENTINEL, "knowledge_base_ids": [note_kb_id]}),
        ("knowledge-chat", "POST", f"/api/v1/knowledge-chat/{bob_session_id}", {"query": SENTINEL, "knowledge_base_ids": [note_kb_id]}),
        ("agent-chat", "POST", f"/api/v1/agent-chat/{bob_session_id}", {"query": SENTINEL, "knowledge_base_ids": [note_kb_id]}),
        ("agent-selection", "POST", "/api/v1/agents", {"name": "denied", "config": {"knowledge_bases": [note_kb_id]}}),
        ("copy", "POST", "/api/v1/knowledge-bases/copy", {"source_id": note_kb_id}),
        ("favorite", "POST", "/api/v1/user/favorites", {"type": "kb", "id": note_kb_id}),
        ("session-guess", "GET", f"/api/v1/sessions/{alice_session_id}", None),
    ]
    for label, method, path, payload in matrix:
        try:
            expect_error(
                client,
                method,
                path,
                404,
                "resource.not_found",
                payload=payload,
                token=bob_shared_token,
            )
        except RuntimeError as error:
            raise RuntimeError(f"matrix row {label} failed: {error}") from error

    report = {
        "owner_read_write_retrieval": "pass",
        "idempotent_mode_creation": "pass",
        "owner_share_publish_policy": "pass",
        "list_filtering": "pass",
        "workspace_admin_bypass_denied": "pass",
        "credential_matrix": "pass",
        "negative_authorization_rows": len(matrix),
        "plain_rag_control_access": "pass",
        "sentinel_not_retrievable_by_non_owner": True,
    }
    report_path = Path(arguments.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(f"Gate B authorization matrix passed: {len(matrix)} negative rows")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Gate B authorization probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
