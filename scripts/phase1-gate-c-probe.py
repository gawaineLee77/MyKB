#!/usr/bin/env python3
"""Run compact live Personal Notes CRUD, quota, authorization, and recovery checks."""

from __future__ import annotations

import argparse
import json
import os
import runpy
import secrets
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
SUPPORT = runpy.run_path(str(ROOT / "scripts/phase1-gate-b-probe.py"))
Client = SUPPORT["Client"]
APIError = SUPPORT["APIError"]
error_code = SUPPORT["error_code"]
expect_error = SUPPORT["expect_error"]
login = SUPPORT["login"]
wait_for_gateway = SUPPORT["wait_for_gateway"]


def multipart_request(
    client: Any,
    path: str,
    filename: str,
    content: bytes,
    token: str,
    allowed: tuple[int, ...],
) -> tuple[int, Any]:
    boundary = "mindcreek-" + secrets.token_hex(12)
    body = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
        "Content-Type: application/octet-stream\r\n\r\n"
    ).encode("ascii") + content + f"\r\n--{boundary}--\r\n".encode("ascii")
    request = urllib.request.Request(
        client.base_url + path,
        data=body,
        headers={
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
            "Content-Type": f"multipart/form-data; boundary={boundary}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            status, raw = response.status, response.read(1024 * 1024 + 1)
    except urllib.error.HTTPError as error:
        status, raw = error.code, error.read(1024 * 1024 + 1)
    if len(raw) > 1024 * 1024:
        raise RuntimeError("note import returned an oversized response")
    payload = json.loads(raw) if raw else {}
    if status not in allowed:
        raise RuntimeError(f"note import returned {status}/{error_code(payload)}")
    return status, payload


def assert_multipart_error(
    client: Any, path: str, filename: str, content: bytes, token: str, status: int, code: str
) -> None:
    actual_status, body = multipart_request(client, path, filename, content, token, (status,))
    if actual_status != status or error_code(body) != code:
        raise RuntimeError(
            f"import {filename} returned {actual_status}/{error_code(body)}, expected {status}/{code}"
        )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--report", default=str(ROOT / ".local/phase1-gate-c-report.json"))
    arguments = parser.parse_args()
    client = Client(arguments.base_url)
    wait_for_gateway(client, arguments.health_timeout)

    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    password = f"GateC-{secrets.token_urlsafe(12)}!"
    users = []
    for role in ("owner", "other"):
        email = f"gate-c-{role}-{nonce}@example.invalid"
        client.request(
            "POST",
            "/api/v1/auth/register",
            {"username": f"gate-c-{role}-{nonce}", "email": email, "password": password},
            allowed=(201,),
        )
        users.append(login(client, email, password))
    owner_token, other_token = str(users[0]["token"]), str(users[1]["token"])

    _, model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"gate-c-embedding-{nonce}",
            "type": "Embedding",
            "source": "remote",
            "description": "Synthetic Gate C model",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "gate-c-not-a-secret",
                "provider": "generic",
                "embedding_parameters": {"dimension": 64, "truncate_prompt_tokens": 0},
            },
        },
        token=owner_token,
        allowed=(201,),
    )
    _, space_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        {
            "mode": "personal_notes",
            "index_profile": "notes_plain",
            "name": f"Gate C Notes {nonce}",
            "description": "Synthetic Gate C owner-only notes",
            "embedding_model_id": str(model_body["data"]["id"]),
            "storage_provider": "local",
        },
        token=owner_token,
        allowed=(201,),
        headers={"Idempotency-Key": f"gate-c-space-{nonce}"},
    )
    kb_id = str(space_body["data"]["knowledge_base_id"])
    base = f"/api/v1/knowledge-bases/{kb_id}/notes"

    original_content = f"# Gate C\n\nSynthetic sentinel GATE_C_{nonce}"
    _, created_body = client.request(
        "POST", base, {"title": "Gate C note", "content": original_content, "status": "publish"}, token=owner_token, allowed=(201,)
    )
    note = created_body["data"]
    note_id, version = str(note["id"]), int(note["version"])
    client.request("GET", base, token=owner_token)
    client.request("GET", f"{base}/{note_id}", token=owner_token)
    expect_error(client, "GET", f"{base}/{note_id}", 404, "resource.not_found", token=other_token)
    expect_error(
        client,
        "PATCH",
        f"{base}/{note_id}",
        409,
        "note.version_conflict",
        payload={"title": "Stale", "content": "must not win", "expected_version": version - 1},
        token=owner_token,
    )

    _, updated_body = client.request(
        "PATCH",
        f"{base}/{note_id}",
        {"title": "Gate C edited", "content": "second version", "status": "publish", "expected_version": version},
        token=owner_token,
    )
    updated_version = int(updated_body["data"]["version"])
    _, revisions_body = client.request("GET", f"{base}/{note_id}/revisions", token=owner_token)
    versions = {int(item["version"]) for item in revisions_body["data"]}
    if not {version, updated_version}.issubset(versions):
        raise RuntimeError("revision history is incomplete")
    _, preview_body = client.request("GET", f"{base}/{note_id}/revisions/{version}", token=owner_token)
    if preview_body["data"].get("content") != original_content:
        raise RuntimeError("prior revision preview changed")
    _, restored_body = client.request(
        "POST",
        f"{base}/{note_id}/restore",
        {"expected_version": updated_version, "target_version": version},
        token=owner_token,
    )
    if restored_body["data"].get("content") != original_content or int(restored_body["data"]["version"]) <= updated_version:
        raise RuntimeError("revision restore did not create a newer version")

    import_path = f"{base}/import"
    _, imported_body = multipart_request(client, import_path, "gate-c.md", b"# Imported\n\nsynthetic", owner_token, (201,))
    imported_id = str(imported_body["data"]["id"])
    assert_multipart_error(client, import_path, "blocked.pdf", b"not a pdf", owner_token, 415, "note.file_type_unsupported")
    assert_multipart_error(client, import_path, "invalid.txt", b"\xff\xfe", owner_token, 400, "note.invalid_utf8")
    assert_multipart_error(client, import_path, "large.md", b"x" * (64 * 1024 + 1), owner_token, 413, "note.size_quota_exceeded")

    client.request("DELETE", f"{base}/{imported_id}", token=owner_token)
    client.request("DELETE", f"{base}/{note_id}", token=owner_token)
    delete_deadline = time.time() + 60
    while True:
        status, deleted_body = client.request(
            "GET", f"{base}/{note_id}", token=owner_token, allowed=(200, 404)
        )
        if status == 404 and error_code(deleted_body) == "note.not_found":
            break
        if time.time() >= delete_deadline:
            raise RuntimeError("asynchronous note deletion did not complete")
        time.sleep(1)

    report = {
        "owner_crud": "pass",
        "non_owner_denied": "pass",
        "markdown_text_import": "pass",
        "format_utf8_size_preflight": "pass",
        "optimistic_concurrency": "pass",
        "revision_preview_restore": "pass",
        "delete": "pass",
    }
    report_path = Path(arguments.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Gate C passed: owner CRUD, import policy, quotas, authorization, and recovery")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Gate C probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
