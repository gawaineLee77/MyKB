#!/usr/bin/env python3
"""Small live probe for an operator-configured Phase 5 Gate B deployment."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request


BASE = os.environ.get("MINDCREEK_PROBE_URL", "http://127.0.0.1:18080").rstrip("/")


def request(path: str, method: str = "GET") -> tuple[int, dict]:
    req = urllib.request.Request(BASE + path, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        try:
            payload = json.load(error)
        except Exception:
            payload = {}
        return error.code, payload


status, broker = request("/api/v1/mindcreek/oidc/status")
assert status == 200 and broker.get("enabled") is True and broker.get("registration") == "closed", broker
assert broker.get("corporate_protocol") in {"oauth2", "oidc"}, broker
if broker.get("corporate_protocol") == "oauth2":
    assert broker.get("authorization_method") in {"GET", "POST"}, broker
    assert broker.get("userinfo_token_transport") in {"bearer", "query"}, broker

status, closed = request("/api/v1/auth/login", "POST")
assert status == 404 and closed.get("error", {}).get("code") == "identity.closed_registration", closed

status, oidc = request("/api/v1/auth/oidc/config")
assert status == 200 and oidc.get("success") is True and oidc.get("enabled") is True, oidc

callback = urllib.parse.quote(BASE + "/api/v1/auth/oidc/callback", safe="")
status, authorization = request("/api/v1/auth/oidc/url?redirect_uri=" + callback)
assert status == 200 and authorization.get("success") is True, authorization
target = urllib.parse.urlparse(authorization.get("authorization_url", ""))
assert target.path == "/api/v1/mindcreek/oidc/authorize", target.geturl()
query = urllib.parse.parse_qs(target.query)
assert query.get("response_type") == ["code"] and query.get("state"), query

status, discovery = request("/api/v1/mindcreek/oidc/.well-known/openid-configuration")
assert status == 200 and discovery.get("id_token_signing_alg_values_supported") == ["RS256"], discovery
assert "client_secret" not in json.dumps(discovery).lower(), discovery

print("MindCreek Phase 5 Gate B live identity configuration verified")
