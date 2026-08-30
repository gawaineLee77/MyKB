#!/usr/bin/env python3
"""Synthetic Phase 4 fixture helpers shared by live Web and MCP probes."""

from __future__ import annotations

import runpy
import secrets
import time
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
RAG_SUPPORT = runpy.run_path(str(ROOT / "scripts/phase1-gate-d-probe.py"))
Client = RAG_SUPPORT["Client"]
APIError = RAG_SUPPORT["APIError"]
login = RAG_SUPPORT["login"]
multipart_request = RAG_SUPPORT["multipart_request"]
wait_for_documents = RAG_SUPPORT["wait_for_documents"]
wait_for_gateway = RAG_SUPPORT["wait_for_gateway"]


def create_fixture(client: Any, prefix: str = "phase4") -> dict[str, Any]:
    """Create two users and three synthetic KBs in one tenant."""
    nonce = f"{int(time.time())}-{secrets.token_hex(3)}"
    password = f"Phase4-{secrets.token_urlsafe(12)}!"
    principals: dict[str, dict[str, Any]] = {}
    for name in ("alice", "bob"):
        email = f"{prefix}-{name}-{nonce}@example.invalid"
        client.request(
            "POST",
            "/api/v1/auth/register",
            {"username": f"{prefix}-{name}-{nonce}", "email": email, "password": password},
            allowed=(201,),
        )
        principals[name] = login(client, email, password)

    alice, bob = principals["alice"], principals["bob"]
    alice_token = str(alice["token"])
    bob_original_token = str(bob["token"])
    tenant_id = int(alice["active_tenant"]["id"])
    bob_user_id = str(bob["user"]["id"])

    _, embedding = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"{prefix}-embedding-{nonce}",
            "type": "Embedding",
            "source": "remote",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "synthetic",
                "provider": "generic",
                "embedding_parameters": {"dimension": 64, "truncate_prompt_tokens": 0},
            },
        },
        token=alice_token,
        allowed=(201,),
    )
    _, chat = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"{prefix}-chat-{nonce}",
            "type": "KnowledgeQA",
            "source": "remote",
            "is_default": True,
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "synthetic",
                "provider": "generic",
                "temperature": 0,
            },
        },
        token=alice_token,
        allowed=(201,),
    )
    _, rerank = client.request(
        "POST",
        "/api/v1/models",
        {
            "name": f"{prefix}-rerank-{nonce}",
            "type": "Rerank",
            "source": "remote",
            "parameters": {
                "base_url": "http://mock-embedding:19090/v1",
                "api_key": "synthetic",
                "provider": "generic",
            },
        },
        token=alice_token,
        allowed=(201,),
    )
    embedding_id = str(embedding["data"]["id"])
    chat_id = str(chat["data"]["id"])
    rerank_id = str(rerank["data"]["id"])
    _, reasoning_agent = client.request(
        "GET",
        "/api/v1/agents/builtin-smart-reasoning",
        token=alice_token,
    )
    reasoning_config = dict(reasoning_agent["data"]["config"])
    reasoning_config.update({
        "model_id": chat_id,
        "rerank_model_id": rerank_id,
        "web_search_enabled": False,
    })
    client.request(
        "PUT",
        "/api/v1/agents/builtin-smart-reasoning",
        {
            "name": reasoning_agent["data"]["name"],
            "description": reasoning_agent["data"].get("description", ""),
            "avatar": reasoning_agent["data"].get("avatar", ""),
            "config": reasoning_config,
        },
        token=alice_token,
    )

    kb_ids: dict[str, str] = {}
    document_ids: dict[str, str] = {}
    sentinels = {
        "shared": f"MINDCREEK_SHARED_{nonce}",
        "private": f"MINDCREEK_PRIVATE_{nonce}",
        "public": f"MINDCREEK_PUBLIC_{nonce}",
    }
    for kind in ("shared", "private", "public"):
        _, space = client.request(
            "POST",
            "/api/v1/knowledge-spaces",
            {
                "mode": "rag",
                "index_profile": "plain",
                "name": f"Phase 4 {kind.title()} {nonce}",
                "embedding_model_id": embedding_id,
                "summary_model_id": chat_id,
                "storage_provider": "local",
            },
            token=alice_token,
            allowed=(201,),
            headers={"Idempotency-Key": f"{prefix}-{kind}-{nonce}"},
        )
        kb_id = str(space["data"]["knowledge_base_id"])
        kb_ids[kind] = kb_id
        content = (
            f"# Phase 4 {kind}\n\nEnglish retrieval marker: {sentinels[kind]}.\n\n"
            f"中文检索标记：{sentinels[kind]}，河流知识用于合成评测。"
        ).encode("utf-8")
        ingestion_path = f"/api/v1/knowledge-bases/{kb_id}/ingestions"
        _, upload = multipart_request(client, ingestion_path, f"{kind}.md", content, alice_token, (202,))
        document_id = str(upload["data"]["id"])
        document_ids[kind] = document_id
        completed = wait_for_documents(client, ingestion_path, alice_token, {document_id})
        if completed[document_id].get("parse_status") != "completed":
            raise RuntimeError(f"Phase 4 {kind} fixture did not complete")

    _, invitation = client.request(
        "POST",
        f"/api/v1/tenants/{tenant_id}/invitations",
        {"email": bob["user"]["email"], "role": "admin", "message": "Phase 4 matrix"},
        token=alice_token,
        allowed=(201,),
    )
    client.request(
        "POST",
        f"/api/v1/me/invitations/{int(invitation['data']['id'])}/accept",
        token=bob_original_token,
    )
    _, switched = client.request(
        "POST",
        "/api/v1/auth/switch-tenant",
        {"tenant_id": tenant_id, "refresh_token": bob.get("refresh_token", "")},
        token=bob_original_token,
    )
    bob_token = str(switched["token"])

    _, grant = client.request(
        "POST",
        f"/api/v1/mindcreek/knowledge-bases/{kb_ids['shared']}/grants",
        {"subject_type": "user", "subject_id": bob_user_id, "permission": "viewer"},
        token=alice_token,
        allowed=(201,),
        headers={"X-Request-ID": f"{prefix}-grant-{nonce}"},
    )
    _, publication = client.request(
        "POST",
        f"/api/v1/mindcreek/knowledge-bases/{kb_ids['public']}/publication",
        {
            "title": f"Phase 4 Public {nonce}",
            "description": "Synthetic organization-public scope fixture",
            "tags": ["phase4", "scope"],
            "usage_guidance": "Synthetic verification only",
            "audience": {"type": "organization"},
            "access_mode": "organization_public",
        },
        token=alice_token,
        allowed=(201,),
        headers={"X-Request-ID": f"{prefix}-publish-{nonce}"},
    )
    return {
        "nonce": nonce,
        "tenant_id": tenant_id,
        "alice_token": alice_token,
        "bob_token": bob_token,
        "bob_original_token": bob_original_token,
        "bob_user_id": bob_user_id,
        "chat_model_id": chat_id,
        "rerank_model_id": rerank_id,
        "kb_ids": kb_ids,
        "document_ids": document_ids,
        "sentinels": sentinels,
        "grant": grant["data"],
        "publication": publication["data"],
    }
