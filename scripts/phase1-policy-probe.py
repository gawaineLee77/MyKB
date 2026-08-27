#!/usr/bin/env python3
"""Probe the live Phase 1 gateway through the browser-facing frontend."""

import json
import os
import sys
import urllib.error
import urllib.request


BASE_URL = os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080").rstrip("/")
DISABLED_PATHS = (
    "/api/v1/im/channels",
    "/api/v1/agents/probe-agent/im-channels",
    "/api/v1/mcp-services",
    "/api/v1/web-search/providers",
    "/api/v1/datasource",
    "/api/v1/embed/probe",
    "/api/v1/evaluation",
    "/files",
    "/r/probe-token",
)


def request_json(path: str):
    try:
        with urllib.request.urlopen(BASE_URL + path, timeout=10) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        with error:
            return error.code, json.load(error)


def request_text(path: str):
    with urllib.request.urlopen(BASE_URL + path, timeout=10) as response:
        return response.status, response.read(64 * 1024).decode("utf-8", errors="replace")


def fail(message: str):
    print(f"MindCreek Phase 1 policy probe failed: {message}", file=sys.stderr)
    raise SystemExit(1)


for disabled_path in DISABLED_PATHS:
    status, body = request_json(disabled_path)
    if status != 404 or body.get("error", {}).get("code") != "feature.disabled":
        fail(f"{disabled_path} returned status={status} body={body!r}")

status, capabilities = request_json("/api/v1/capabilities/knowledge-modes")
flags = capabilities.get("capabilities", {})
if status != 200 or flags.get("rag_plain") is not True:
    fail("Plain RAG is not enabled in the capability document")
if flags.get("kb_personal_notes") is not True:
    fail("Personal Notes is not enabled after Gate B and P1-14")
if any(value for key, value in flags.items() if key not in ("rag_plain", "kb_personal_notes")):
    fail("an excluded or future capability is enabled")

status, swagger_fallback = request_text("/swagger/index.html")
if status != 200 or "<title>MindCreek</title>" not in swagger_fallback:
    fail("Swagger path was exposed instead of remaining inside the frontend SPA")

status, _ = request_json("/api/v1/auth/config")
if status != 200:
    fail(f"permitted auth/config route returned {status}")

print(f"MindCreek Phase 1 policy verified through {BASE_URL}: {len(DISABLED_PATHS)} disabled probes")
