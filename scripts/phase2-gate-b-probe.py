#!/usr/bin/env python3
"""Run the live Phase 2 explicit-sharing and revocation matrix."""

from __future__ import annotations

import argparse
import datetime
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
SUPPORT = runpy.run_path(str(ROOT / "scripts/phase1-gate-d-probe.py"))
Client = SUPPORT["Client"]
APIError = SUPPORT["APIError"]
error_code = SUPPORT["error_code"]
expect_error = runpy.run_path(str(ROOT / "scripts/phase1-gate-b-probe.py"))["expect_error"]
hybrid_search = SUPPORT["hybrid_search"]
login = SUPPORT["login"]
multipart_request = SUPPORT["multipart_request"]
normal_chat = SUPPORT["normal_chat"]
verify_openable_citations = SUPPORT["verify_openable_citations"]
wait_for_documents = SUPPORT["wait_for_documents"]
wait_for_gateway = SUPPORT["wait_for_gateway"]


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def audit_counts(correlations: list[str]) -> tuple[int, int]:
    values = load_env(ROOT / ".local/mindcreek.env")
    user, database = values.get("DB_USER", ""), values.get("DB_NAME", "")
    if not user.replace("_", "").isalnum() or not database.replace("_", "").isalnum():
        raise RuntimeError("runtime database identifiers are invalid")
    quoted = ",".join("'" + value.replace("'", "''") + "'" for value in correlations)
    sql = (
        "SELECT concat(count(*), '|', count(*) FILTER (WHERE "
        "coalesce(old_value::text, '') ~* '(content|prompt|document)' OR "
        "coalesce(new_value::text, '') ~* '(content|prompt|document)')) "
        "FROM mindcreek.kb_access_audit_events WHERE correlation_id IN ("
        + quoted
        + ")"
    )
    result = subprocess.run(
        [
            "docker",
            "exec",
            "MindCreek-postgres",
            "psql",
            "-U",
            user,
            "-d",
            database,
            "-Atqc",
            sql,
        ],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError("unable to verify Phase 2 audit events")
    parts = result.stdout.strip().split("|")
    if len(parts) != 2:
        raise RuntimeError("Phase 2 audit query returned an invalid result")
    return int(parts[0]), int(parts[1])


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--report", default=str(ROOT / ".local/phase2-gate-b-report.json"))
    arguments = parser.parse_args()
    client = Client(arguments.base_url)
    wait_for_gateway(client, arguments.health_timeout)

    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    password = f"Phase2-{secrets.token_urlsafe(12)}!"
    principals: dict[str, dict[str, Any]] = {}
    for name in ("alice", "bob"):
        email = f"phase2-{name}-{nonce}@example.invalid"
        client.request(
            "POST",
            "/api/v1/auth/register",
            {"username": f"phase2-{name}-{nonce}", "email": email, "password": password},
            allowed=(201,),
        )
        principals[name] = login(client, email, password)

    alice, bob = principals["alice"], principals["bob"]
    alice_token, bob_original_token = str(alice["token"]), str(bob["token"])
    alice_tenant_id = int(alice["active_tenant"]["id"])
    bob_user_id = str(bob["user"]["id"])

    _, model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"phase2-embedding-{nonce}",
            "type": "Embedding",
            "source": "remote",
            "description": "Synthetic Phase 2 embedding model",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "phase2-not-a-secret",
                "provider": "generic",
                "embedding_parameters": {"dimension": 64, "truncate_prompt_tokens": 0},
            },
        },
        token=alice_token,
        allowed=(201,),
    )
    _, chat_model_body = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"phase2-chat-{nonce}",
            "type": "KnowledgeQA",
            "source": "remote",
            "description": "Synthetic Phase 2 chat model",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "phase2-not-a-secret",
                "provider": "generic",
                "temperature": 0,
            },
            "is_default": True,
        },
        token=alice_token,
        allowed=(201,),
    )
    model_id, chat_model_id = str(model_body["data"]["id"]), str(chat_model_body["data"]["id"])

    _, rag_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        {
            "mode": "rag",
            "index_profile": "plain",
            "name": f"Phase 2 Shared RAG {nonce}",
            "description": "Synthetic explicit-sharing resource",
            "embedding_model_id": model_id,
            "summary_model_id": chat_model_id,
            "storage_provider": "local",
        },
        token=alice_token,
        allowed=(201,),
        headers={"Idempotency-Key": f"phase2-rag-{nonce}"},
    )
    rag_kb_id = str(rag_body["data"]["knowledge_base_id"])
    _, notes_body = client.request(
        "POST",
        "/api/v1/knowledge-spaces",
        {
            "mode": "personal_notes",
            "index_profile": "notes_plain",
            "name": f"Phase 2 Private Notes {nonce}",
            "embedding_model_id": model_id,
            "storage_provider": "local",
        },
        token=alice_token,
        allowed=(201,),
        headers={"Idempotency-Key": f"phase2-notes-{nonce}"},
    )
    notes_kb_id = str(notes_body["data"]["knowledge_base_id"])

    sentinel = f"MINDCREEK_PHASE2_{nonce}"
    ingestion_path = f"/api/v1/knowledge-bases/{rag_kb_id}/ingestions"
    _, upload_body = multipart_request(
        client,
        ingestion_path,
        "phase2.md",
        f"# Explicit sharing\n\n{sentinel}".encode(),
        alice_token,
        (202,),
    )
    document_id = str(upload_body["data"]["id"])
    documents = wait_for_documents(client, ingestion_path, alice_token, {document_id})
    if documents[document_id].get("parse_status") != "completed":
        raise RuntimeError("Phase 2 synthetic document did not complete")

    _, invitation_body = client.request(
        "POST",
        f"/api/v1/tenants/{alice_tenant_id}/invitations",
        {"email": bob["user"]["email"], "role": "admin", "message": "Phase 2 matrix"},
        token=alice_token,
        allowed=(201,),
    )
    client.request("POST", f"/api/v1/me/invitations/{int(invitation_body['data']['id'])}/accept", token=bob_original_token)
    _, switched = client.request(
        "POST",
        "/api/v1/auth/switch-tenant",
        {"tenant_id": alice_tenant_id, "refresh_token": bob.get("refresh_token", "")},
        token=bob_original_token,
    )
    bob_token = str(switched["token"])

    expect_error(client, "GET", f"/api/v1/knowledge-bases/{rag_kb_id}", 404, "resource.not_found", token=bob_token)
    expect_error(client, "GET", f"/api/v1/knowledge-bases/{rag_kb_id}", 404, "resource.not_found", token=bob_original_token)
    _, peer_list = client.request("GET", "/api/v1/knowledge-bases", token=bob_token)
    if rag_kb_id in {str(item.get("id")) for item in peer_list.get("data", [])}:
        raise RuntimeError("same-workspace peer list exposed a private KB")

    expect_error(
        client,
        "POST",
        f"/api/v1/mindcreek/knowledge-bases/{notes_kb_id}/grants",
        403,
        "personal_notes.sharing_disabled",
        payload={"subject_type": "user", "subject_id": bob_user_id, "permission": "viewer"},
        token=alice_token,
        headers={"X-Request-ID": f"phase2-notes-denied-{nonce}"},
    )
    expect_error(
        client,
        "POST",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants",
        400,
        "grant.subject_unsupported",
        payload={"subject_type": "group", "subject_id": "group-1", "permission": "viewer"},
        token=alice_token,
    )

    create_correlation = f"phase2-grant-create-{nonce}"
    _, grant_body = client.request(
        "POST",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants",
        {"subject_type": "user", "subject_id": bob_user_id, "permission": "viewer"},
        token=alice_token,
        allowed=(201,),
        headers={"X-Request-ID": create_correlation},
    )
    active_grant = grant_body["data"]
    grant_id, revision = str(active_grant["id"]), int(active_grant["revision"])

    _, shared_page = client.request("GET", "/api/v1/mindcreek/knowledge-bases?view=shared", token=bob_token)
    shared_items = shared_page.get("data", {}).get("items", [])
    if not any(item.get("id") == rag_kb_id and item.get("role") == "viewer" for item in shared_items):
        raise RuntimeError("Viewer grant did not appear in Shared with me")
    _, viewer_access_body = client.request(
        "GET", f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/access", token=bob_token
    )
    viewer_access = viewer_access_body.get("data", {})
    if viewer_access.get("role") != "viewer" or viewer_access.get("can_edit_content") is not False:
        raise RuntimeError("server access summary did not render Viewer restrictions")
    client.request("GET", f"/api/v1/knowledge-bases/{rag_kb_id}", token=bob_token)
    client.request("GET", f"/api/v1/knowledge-bases/{rag_kb_id}/product-profile", token=bob_token)
    client.request("GET", ingestion_path, token=bob_token)
    hybrid_search(client, rag_kb_id, bob_token, sentinel, sentinel)

    _, viewer_session_body = client.request(
        "POST",
        "/api/v1/sessions",
        {"title": "Phase 2 Viewer", "description": "Revocation scope probe"},
        token=bob_token,
        allowed=(201,),
    )
    viewer_session_id = str(viewer_session_body["data"]["id"])
    chat_events = normal_chat(client, bob_token, viewer_session_id, rag_kb_id, chat_model_id, sentinel)
    if verify_openable_citations(client, bob_token, rag_kb_id, chat_events, sentinel) < 1:
        raise RuntimeError("Viewer received no openable citation")
    references = [reference for event in chat_events for reference in event.get("knowledge_references", [])]
    citation = next(reference for reference in references if sentinel in str(reference.get("content", "")))
    citation_knowledge_id, citation_chunk_id = str(citation["knowledge_id"]), str(citation["id"])

    viewer_delete_correlation = f"phase2-viewer-delete-{nonce}"
    expect_error(
        client,
        "DELETE",
        f"/api/v1/knowledge-bases/{rag_kb_id}",
        404,
        "resource.not_found",
        token=bob_token,
        headers={"X-Request-ID": viewer_delete_correlation},
    )
    expect_error(
        client,
        "PUT",
        f"/api/v1/knowledge-bases/{rag_kb_id}",
        404,
        "resource.not_found",
        payload={"name": "Viewer denied", "description": "Must not change"},
        token=bob_token,
    )
    upload_status, upload_error = multipart_request(client, ingestion_path, "viewer.md", b"denied", bob_token, (404,))
    if upload_status != 404 or error_code(upload_error) != "resource.not_found":
        raise RuntimeError("Viewer upload was not denied")
    grant_denied_correlation = f"phase2-grants-denied-{nonce}"
    expect_error(
        client,
        "GET",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants",
        404,
        "resource.not_found",
        token=bob_token,
        headers={"X-Request-ID": grant_denied_correlation},
    )

    update_correlation = f"phase2-grant-update-{nonce}"
    _, updated_body = client.request(
        "PATCH",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants/{grant_id}",
        {"expected_revision": revision, "permission": "editor"},
        token=alice_token,
        headers={"X-Request-ID": update_correlation},
    )
    revision = int(updated_body["data"]["revision"])
    _, editor_access_body = client.request(
        "GET", f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/access", token=bob_token
    )
    editor_access = editor_access_body.get("data", {})
    if editor_access.get("role") != "editor" or editor_access.get("can_edit_content") is not True or editor_access.get("can_manage_grants") is not False:
        raise RuntimeError("server access summary did not render Editor boundaries")
    expect_error(
        client,
        "PATCH",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants/{grant_id}",
        409,
        "grant.revision_conflict",
        payload={"expected_revision": revision - 1, "permission": "viewer"},
        token=alice_token,
    )

    client.request(
        "PUT",
        f"/api/v1/knowledge-bases/{rag_kb_id}",
        {"name": f"Phase 2 Editor Metadata {nonce}", "description": "Editor-safe metadata"},
        token=bob_token,
    )
    _, editor_upload = multipart_request(client, ingestion_path, "editor.md", b"editor content", bob_token, (202,))
    if not editor_upload.get("data", {}).get("id"):
        raise RuntimeError("Editor content upload did not succeed")
    expect_error(
        client,
        "PUT",
        f"/api/v1/knowledge-bases/{rag_kb_id}",
        404,
        "resource.not_found",
        payload={"name": "Unsafe", "config": {"chunking_config": {"chunk_size": 1}}},
        token=bob_token,
    )
    expect_error(client, "DELETE", f"/api/v1/knowledge-bases/{rag_kb_id}", 404, "resource.not_found", token=bob_token)

    revoke_correlation = f"phase2-grant-revoke-{nonce}"
    _, revoked_body = client.request(
        "DELETE",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants/{grant_id}",
        {"expected_revision": revision},
        token=alice_token,
        headers={"X-Request-ID": revoke_correlation},
    )
    if not revoked_body.get("data", {}).get("revoked_at"):
        raise RuntimeError("grant revocation did not return a timestamp")
    for path in (
        f"/api/v1/knowledge-bases/{rag_kb_id}",
        f"/api/v1/knowledge/{citation_knowledge_id}",
        f"/api/v1/chunks/by-id/{citation_chunk_id}",
        f"/api/v1/messages/{viewer_session_id}/load",
    ):
        expect_error(client, "GET", path, 404, "resource.not_found", token=bob_token)

    expiry_correlation = f"phase2-grant-expiry-{nonce}"
    expires_at = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(seconds=5)
    client.request(
        "POST",
        f"/api/v1/mindcreek/knowledge-bases/{rag_kb_id}/grants",
        {
            "subject_type": "user",
            "subject_id": bob_user_id,
            "permission": "viewer",
            "expires_at": expires_at.isoformat().replace("+00:00", "Z"),
        },
        token=alice_token,
        allowed=(201,),
        headers={"X-Request-ID": expiry_correlation},
    )
    client.request("GET", f"/api/v1/knowledge-bases/{rag_kb_id}", token=bob_token)
    time.sleep(max(0.0, expires_at.timestamp() - time.time()) + 1.0)
    expect_error(client, "GET", f"/api/v1/knowledge-bases/{rag_kb_id}", 404, "resource.not_found", token=bob_token)
    _, expired_page = client.request("GET", "/api/v1/mindcreek/knowledge-bases?view=shared", token=bob_token)
    if any(item.get("id") == rag_kb_id for item in expired_page.get("data", {}).get("items", [])):
        raise RuntimeError("expired grant remained in Shared with me")

    correlations = [
        create_correlation,
        update_correlation,
        revoke_correlation,
        expiry_correlation,
        viewer_delete_correlation,
        grant_denied_correlation,
        f"phase2-notes-denied-{nonce}",
    ]
    event_count, sensitive_count = audit_counts(correlations)
    if event_count < len(correlations) or sensitive_count != 0:
        raise RuntimeError(f"audit verification failed: events={event_count} sensitive={sensitive_count}")

    report = {
        "private_by_default": "pass",
        "same_workspace_admin_bypass": "denied",
        "viewer_read_search_chat_citations": "pass",
        "viewer_mutations": "denied",
        "editor_content_and_metadata": "pass",
        "editor_grants_config_delete": "denied",
        "personal_notes_sharing": "denied",
        "revision_precondition": "pass",
        "revocation_citations_sessions": "pass",
        "expiry": "pass",
        "audit_events": event_count,
        "sensitive_audit_payloads": sensitive_count,
    }
    report_path = Path(arguments.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Phase 2 Gate B passed: private sharing, roles, revocation, expiry, and audit")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Phase 2 Gate B probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
