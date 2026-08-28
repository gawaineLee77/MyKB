#!/usr/bin/env python3
"""Run the live Phase 3 publication, catalog, and subscription matrix."""

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
SUPPORT = runpy.run_path(str(ROOT / "scripts/phase1-gate-d-probe.py"))
Client = SUPPORT["Client"]
APIError = SUPPORT["APIError"]
expect_error = runpy.run_path(str(ROOT / "scripts/phase1-gate-b-probe.py"))["expect_error"]
hybrid_search = SUPPORT["hybrid_search"]
login = SUPPORT["login"]
multipart_request = SUPPORT["multipart_request"]
wait_for_documents = SUPPORT["wait_for_documents"]
wait_for_gateway = SUPPORT["wait_for_gateway"]


def load_env(path: Path) -> dict[str, str]:
    result: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            result[key.strip()] = value.strip().strip('"').strip("'")
    return result


def audit_count(correlations: list[str]) -> tuple[int, int]:
    values = load_env(ROOT / ".local/mindcreek.env")
    user, database = values.get("DB_USER", ""), values.get("DB_NAME", "")
    quoted = ",".join("'" + value.replace("'", "''") + "'" for value in correlations)
    sql = (
        "SELECT concat(count(*),'|',count(*) FILTER (WHERE "
        "coalesce(old_value::text,'') ~* '(content|prompt|document)' OR "
        "coalesce(new_value::text,'') ~* '(content|prompt|document)')) "
        "FROM mindcreek.kb_access_audit_events WHERE correlation_id IN (" + quoted + ")"
    )
    result = subprocess.run(
        ["docker", "exec", "MindCreek-postgres", "psql", "-U", user, "-d", database, "-Atqc", sql],
        cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
    )
    if result.returncode != 0 or "|" not in result.stdout:
        raise RuntimeError("unable to verify Phase 3 audit records")
    total, sensitive = result.stdout.strip().split("|", 1)
    return int(total), int(sensitive)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--health-timeout", type=int, default=180)
    parser.add_argument("--report", default=str(ROOT / ".local/phase3-gate-b-report.json"))
    args = parser.parse_args()
    client = Client(args.base_url)
    wait_for_gateway(client, args.health_timeout)
    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    password = f"Phase3-{secrets.token_urlsafe(12)}!"
    principals: dict[str, dict[str, Any]] = {}
    for name in ("alice", "bob"):
        email = f"phase3-{name}-{nonce}@example.invalid"
        client.request("POST", "/api/v1/auth/register", {"username": f"phase3-{name}-{nonce}", "email": email, "password": password}, allowed=(201,))
        principals[name] = login(client, email, password)
    alice, bob = principals["alice"], principals["bob"]
    alice_token, bob_original_token = str(alice["token"]), str(bob["token"])
    tenant_id, bob_user_id = int(alice["active_tenant"]["id"]), str(bob["user"]["id"])

    _, embedding = client.request("POST", "/api/v1/models", {
        "name": f"phase3-embedding-{nonce}", "type": "Embedding", "source": "remote",
        "parameters": {"base_url": "http://mock-embedding:19090/v1", "api_key": "synthetic", "provider": "generic", "embedding_parameters": {"dimension": 64, "truncate_prompt_tokens": 0}},
    }, token=alice_token, allowed=(201,))
    _, chat = client.request("POST", "/api/v1/models", {
        "name": f"phase3-chat-{nonce}", "type": "KnowledgeQA", "source": "remote", "is_default": True,
        "parameters": {"base_url": "http://mock-embedding:19090/v1", "api_key": "synthetic", "provider": "generic", "temperature": 0},
    }, token=alice_token, allowed=(201,))
    embedding_id, chat_id = str(embedding["data"]["id"]), str(chat["data"]["id"])
    _, rag = client.request("POST", "/api/v1/knowledge-spaces", {
        "mode": "rag", "index_profile": "plain", "name": f"Phase 3 Publication {nonce}",
        "embedding_model_id": embedding_id, "summary_model_id": chat_id, "storage_provider": "local",
    }, token=alice_token, allowed=(201,), headers={"Idempotency-Key": f"phase3-rag-{nonce}"})
    _, notes = client.request("POST", "/api/v1/knowledge-spaces", {
        "mode": "personal_notes", "index_profile": "notes_plain", "name": f"Phase 3 Notes {nonce}",
        "embedding_model_id": embedding_id, "storage_provider": "local",
    }, token=alice_token, allowed=(201,), headers={"Idempotency-Key": f"phase3-notes-{nonce}"})
    kb_id, notes_id = str(rag["data"]["knowledge_base_id"]), str(notes["data"]["knowledge_base_id"])
    sentinel = f"MINDCREEK_PHASE3_{nonce}"
    ingestion_path = f"/api/v1/knowledge-bases/{kb_id}/ingestions"
    _, upload = multipart_request(client, ingestion_path, "phase3.md", f"# Published knowledge\n\n{sentinel}".encode(), alice_token, (202,))
    document_id = str(upload["data"]["id"])
    if wait_for_documents(client, ingestion_path, alice_token, {document_id})[document_id].get("parse_status") != "completed":
        raise RuntimeError("Phase 3 document did not complete")

    _, invitation = client.request("POST", f"/api/v1/tenants/{tenant_id}/invitations", {"email": bob["user"]["email"], "role": "admin", "message": "Phase 3 matrix"}, token=alice_token, allowed=(201,))
    client.request("POST", f"/api/v1/me/invitations/{int(invitation['data']['id'])}/accept", token=bob_original_token)
    _, switched = client.request("POST", "/api/v1/auth/switch-tenant", {"tenant_id": tenant_id, "refresh_token": bob.get("refresh_token", "")}, token=bob_original_token)
    bob_token = str(switched["token"])
    expect_error(client, "GET", f"/api/v1/knowledge-bases/{kb_id}", 404, "resource.not_found", token=bob_token)

    notes_correlation = f"phase3-notes-denied-{nonce}"
    expect_error(client, "POST", f"/api/v1/mindcreek/knowledge-bases/{notes_id}/publication", 403, "personal_notes.publication_disabled", payload={
        "title": "No", "audience": {"type": "organization"}, "access_mode": "subscriber",
    }, token=alice_token, headers={"X-Request-ID": notes_correlation})

    publish_correlation = f"phase3-publish-{nonce}"
    _, published = client.request("POST", f"/api/v1/mindcreek/knowledge-bases/{kb_id}/publication", {
        "title": f"Published Guide {nonce}", "description": "Synthetic internal catalog entry", "tags": ["phase3", "guide"],
        "usage_guidance": "Use for synthetic verification only", "audience": {"type": "organization"}, "access_mode": "subscriber",
    }, token=alice_token, allowed=(201,), headers={"X-Request-ID": publish_correlation})
    publication = published["data"]
    publication_id, row_version = str(publication["id"]), int(publication["row_version"])
    _, catalog = client.request("GET", "/api/v1/mindcreek/catalog?q=Published+Guide", token=bob_token)
    catalog_item = next((item for item in catalog["data"]["items"] if item["publication"]["id"] == publication_id), None)
    if not catalog_item or catalog_item.get("can_read") is not False:
        raise RuntimeError("subscriber-access catalog decision is incorrect")
    expect_error(client, "GET", f"/api/v1/knowledge-bases/{kb_id}", 404, "resource.not_found", token=bob_token)

    subscribe_correlation = f"phase3-subscribe-{nonce}"
    status, subscribed = client.request("POST", f"/api/v1/mindcreek/publications/{publication_id}/subscription", token=bob_token, allowed=(201,), headers={"X-Request-ID": subscribe_correlation})
    if status != 201 or not subscribed["data"].get("changed"):
        raise RuntimeError("subscription was not created")
    status, retry = client.request("POST", f"/api/v1/mindcreek/publications/{publication_id}/subscription", token=bob_token, allowed=(200,))
    if status != 200 or retry["data"].get("changed"):
        raise RuntimeError("subscribe retry was not idempotent")
    _, access = client.request("GET", f"/api/v1/mindcreek/knowledge-bases/{kb_id}/access", token=bob_token)
    if access["data"].get("access_source") != "subscription" or access["data"].get("can_download") is not False:
        raise RuntimeError("subscriber access summary is incorrect")
    hybrid_search(client, kb_id, bob_token, sentinel, sentinel)
    expect_error(client, "GET", f"/api/v1/knowledge/{document_id}/download", 404, "resource.not_found", token=bob_token)

    _, second_upload = multipart_request(client, ingestion_path, "update.md", b"synthetic update", alice_token, (202,))
    wait_for_documents(client, ingestion_path, alice_token, {str(second_upload["data"]["id"])})
    _, subscriptions = client.request("GET", "/api/v1/mindcreek/me/subscriptions", token=bob_token)
    followed = next(item for item in subscriptions["data"]["items"] if item["publication"]["id"] == publication_id)
    if not followed.get("updated"):
        raise RuntimeError("content revision did not produce an update badge")
    client.request("POST", f"/api/v1/mindcreek/publications/{publication_id}/mark-seen", token=bob_token)

    unsubscribe_correlation = f"phase3-unsubscribe-{nonce}"
    client.request("DELETE", f"/api/v1/mindcreek/publications/{publication_id}/subscription", token=bob_token, headers={"X-Request-ID": unsubscribe_correlation})
    expect_error(client, "GET", f"/api/v1/knowledge-bases/{kb_id}", 404, "resource.not_found", token=bob_token)

    update_correlation = f"phase3-public-update-{nonce}"
    _, updated = client.request("PATCH", f"/api/v1/mindcreek/knowledge-bases/{kb_id}/publication", {
        "title": publication["title"], "description": publication["description"], "tags": publication["tags"],
        "usage_guidance": publication["usage_guidance"], "audience": {"type": "organization"},
        "access_mode": "organization_public", "expected_row_version": row_version,
    }, token=alice_token, headers={"X-Request-ID": update_correlation})
    row_version = int(updated["data"]["row_version"])
    client.request("GET", f"/api/v1/knowledge-bases/{kb_id}", token=bob_token)
    _, public_access = client.request("GET", f"/api/v1/mindcreek/knowledge-bases/{kb_id}/access", token=bob_token)
    if public_access["data"].get("access_source") != "organization_public":
        raise RuntimeError("organization-public access was not activated")

    audience_correlation = f"phase3-audience-loss-{nonce}"
    _, narrowed = client.request("PATCH", f"/api/v1/mindcreek/knowledge-bases/{kb_id}/publication", {
        "title": publication["title"], "description": publication["description"], "tags": publication["tags"],
        "usage_guidance": publication["usage_guidance"], "audience": {"type": "workspace_set", "workspace_ids": [999999]},
        "access_mode": "organization_public", "expected_row_version": row_version,
    }, token=alice_token, headers={"X-Request-ID": audience_correlation})
    row_version = int(narrowed["data"]["row_version"])
    expect_error(client, "GET", f"/api/v1/knowledge-bases/{kb_id}", 404, "resource.not_found", token=bob_token)
    expect_error(client, "GET", f"/api/v1/mindcreek/publications/{publication_id}", 404, "publication.unavailable", token=bob_token)

    unpublish_correlation = f"phase3-unpublish-{nonce}"
    _, ended = client.request("DELETE", f"/api/v1/mindcreek/knowledge-bases/{kb_id}/publication", {"expected_row_version": row_version}, token=alice_token, headers={"X-Request-ID": unpublish_correlation})
    if ended["data"].get("status") != "unpublished":
        raise RuntimeError("unpublish did not return the terminal state")
    expect_error(client, "GET", f"/api/v1/mindcreek/publications/{publication_id}", 404, "publication.unavailable", token=bob_token)

    correlations = [notes_correlation, publish_correlation, subscribe_correlation, unsubscribe_correlation, update_correlation, audience_correlation, unpublish_correlation]
    events, sensitive = audit_count(correlations)
    if events < len(correlations) or sensitive != 0:
        raise RuntimeError(f"Phase 3 audit check failed: events={events} sensitive={sensitive}")
    report = {
        "subscriber_access": "pass", "organization_public_access": "pass", "original_download": "denied",
        "catalog_audience": "pass", "subscribe_retry": "idempotent", "update_badge": "pass",
        "audience_loss": "pass", "unpublish_inactivation": "pass", "personal_notes_publication": "denied",
        "audit_events": events, "sensitive_audit_payloads": sensitive, "bob_user_id": bob_user_id,
    }
    report_path = Path(args.report)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print("Phase 3 Gate B passed: publication, catalog, public access, subscriptions, and inactivation")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (APIError, RuntimeError, OSError, urllib.error.URLError) as error:
        print(f"Phase 3 Gate B probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
