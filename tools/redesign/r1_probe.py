#!/usr/bin/env python3
"""Disposable, real-upstream HTTP probes for R1; never use an existing stack.

Creates its own randomly named Docker Compose project using locally cached images,
an internal network and no published ports. Databases and files are synthetic.
Only this project's containers/volumes are removed in finally. No source patches,
company OAuth, model endpoints, existing runtime env files or existing volumes.
"""
from __future__ import annotations

import base64
import argparse
import datetime
import hashlib
import http.cookies
import json
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.parse

ROOT = Path(__file__).resolve().parents[2]
IMAGES = {"app": "wechatopenai/weknora-app:v0.8.0", "postgres": "paradedb/paradedb:v0.22.2-pg17", "redis": "redis:7-alpine", "identity": "python:3.12-alpine"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--through", choices=("bootstrap", "all"), default="all")
    args = parser.parse_args()
    project = "mindcreek-r1-" + secrets.token_hex(4)
    local = ROOT / ".local/redesign-r1"
    local.mkdir(parents=True, exist_ok=True)
    run = Path(tempfile.mkdtemp(prefix=project + "-", dir=local))
    compose = run / "compose.json"
    base = ["docker", "compose", "--project-name", project, "-f", str(compose)]
    report = {"time": datetime.datetime.now(datetime.timezone.utc).isoformat(), "project": project, "scope": "real v0.8.0 HTTP APIs with synthetic OAuth provider; no product R2 implementation or real browser/IdP/model acceptance", "checks": [], "requests": [], "images": {}}
    def docker(*args):
        result = subprocess.run([*base, *args], text=True, capture_output=True)
        if result.returncode:
            (run / "docker-error.log").write_text(result.stderr)
            raise RuntimeError("Docker command failed; local diagnostics: " + str(run / "docker-error.log"))
        return result.stdout.strip()
    for key, name in IMAGES.items():
        # Pin the cached immutable ID; do not pull a moving tag during the probe.
        report["images"][key] = {"tag": name, "id": subprocess.check_output(["docker", "image", "inspect", name, "--format", "{{.Id}}"], text=True).strip()}
    secret = secrets.token_hex(16)
    password = "R1-" + secrets.token_hex(10) + "!"
    environment = {
        "DB_DRIVER": "postgres", "DB_HOST": "postgres", "DB_PORT": "5432", "DB_USER": "r1", "DB_PASSWORD": secret, "DB_NAME": "r1",
        "REDIS_ADDR": "redis:6379", "RETRIEVE_DRIVER": "postgres", "STORAGE_TYPE": "local", "LOCAL_STORAGE_BASE_DIR": "/data/files",
        "AUTO_MIGRATE": "true", "GIN_MODE": "release", "JWT_SECRET": secrets.token_hex(32), "SYSTEM_AES_KEY": secrets.token_hex(16),
        "OLLAMA_OPTIONAL": "true", "OLLAMA_BASE_URL": "http://identity:18000", "DOCREADER_ADDR": "identity:50051", "NEO4J_ENABLE": "false",
        "WEKNORA_AUTH_DEFAULT_TENANT_MODE": "tenantless", "WEKNORA_TENANT_SELF_SERVICE_CREATION_ENABLED": "true",
        "WEKNORA_TENANT_ENABLE_RBAC": "true", "WEKNORA_TENANT_ENABLE_CROSS_TENANT_ACCESS": "false", "WEKNORA_TENANT_AUTO_CREATE_API_KEY": "false",
        "WEKNORA_BOOTSTRAP_SYSTEM_ADMIN_EMAIL": "admin@example.invalid", "DISABLE_REGISTRATION": "false",
        "OIDC_AUTH_ENABLE": "true", "OIDC_AUTH_CLIENT_ID": "r1", "OIDC_AUTH_CLIENT_SECRET": secret,
        "OIDC_AUTH_ISSUER_URL": "http://identity:18000", "OIDC_AUTH_AUTHORIZATION_ENDPOINT": "http://identity:18000/authorize",
        "OIDC_AUTH_TOKEN_ENDPOINT": "http://identity:18000/token", "OIDC_AUTH_USER_INFO_ENDPOINT": "http://identity:18000/userinfo",
        "OIDC_USER_INFO_MAPPING_USER_NAME": "name", "OIDC_USER_INFO_MAPPING_EMAIL": "email", "SSRF_WHITELIST_EXTRA": "identity",
        "WEKNORA_SANDBOX_DOCKER_ENABLED": "false", "LANGFUSE_ENABLED": "false",
    }
    services = {
        "postgres": {"image": report["images"]["postgres"]["id"], "environment": {"POSTGRES_USER": "r1", "POSTGRES_PASSWORD": secret, "POSTGRES_DB": "r1"}, "tmpfs": ["/var/lib/postgresql/data"], "healthcheck": {"test": ["CMD-SHELL", "pg_isready -U r1 -d r1"], "interval": "2s", "timeout": "2s", "retries": 30}},
        "redis": {"image": report["images"]["redis"]["id"], "command": ["redis-server", "--save", "", "--appendonly", "no"], "tmpfs": ["/data"]},
        "identity": {"image": report["images"]["identity"]["id"], "command": ["python", "/fixture/mock_identity.py"], "volumes": [str(ROOT / "testdata/redesign/mock_identity.py") + ":/fixture/mock_identity.py:ro"]},
        "app": {"image": report["images"]["app"]["id"], "environment": environment, "tmpfs": ["/data/files"], "depends_on": {"postgres": {"condition": "service_healthy"}, "redis": {"condition": "service_started"}, "identity": {"condition": "service_started"}}},
    }
    compose.write_text(json.dumps({"services": services, "networks": {"default": {"internal": True}}}))
    compose.chmod(0o600)
    origin = "http://app:8080"
    cookies = {}
    transport = '''import json,sys,urllib.request,urllib.error
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*a): return None
p=json.load(sys.stdin)
req=urllib.request.Request(p['url'],data=None if p['body'] is None else json.dumps(p['body']).encode(),method=p['method'],headers=p['headers'])
try:
 try: r=urllib.request.build_opener(urllib.request.ProxyHandler({}),NoRedirect()).open(req,timeout=12)
 except urllib.error.HTTPError as e: r=e
 raw=r.read()
 print(json.dumps({'status':r.code,'headers':dict(r.headers),'body':json.loads(raw) if raw.startswith((b'{',b'[')) else {}}))
except OSError:
 print(json.dumps({'connection_error':True}))
'''
    def request(path, method="GET", body=None, token=None, tenant=None, headers=None, expected=(200,)):
        hdr = {"Content-Type": "application/json", **(headers or {})}
        if cookies: hdr["Cookie"] = "; ".join(k + "=" + v for k, v in cookies.items())
        if token: hdr["Authorization"] = "Bearer " + token
        if tenant: hdr["X-Tenant-ID"] = str(tenant)
        result = subprocess.run([*base, "exec", "-T", "identity", "python", "-c", transport], input=json.dumps({"url": origin + path, "method": method, "body": body, "headers": hdr}), capture_output=True, text=True, check=True)
        response = json.loads(result.stdout)
        if response.get("connection_error"): raise OSError("app not ready")
        data, status, response_headers = response["body"], response["status"], response["headers"]
        if path != "/health":
            auth = "embed" if hdr.get("Authorization", "").startswith("Embed ") else "bearer" if token else "member_api_key" if hdr.get("X-API-Key") else "none"
            report["requests"].append({"method": method, "path": path.split("?")[0], "auth": auth, "workspace_header": bool(tenant), "expected_status": list(expected), "actual_status": status})
        parsed = http.cookies.SimpleCookie(response_headers.get("Set-Cookie", ""))
        for key, value in parsed.items(): cookies[key] = value.value
        if status not in expected:
            # Never include successful auth/key payloads in logs or reports.
            raise AssertionError(f"{method} {path.split('?')[0]}: status {status}; expected {expected}; error {str(data.get('error', data.get('message', '')))[:240]}")
        return data, response_headers, status
    def check(name, condition=True, **facts):
        if not condition: raise AssertionError(name)
        report["checks"].append({"name": name, "passed": True, **facts})
        print("PASS " + name, flush=True)
    def wait_app():
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            try:
                request("/health")
                return
            except (OSError, AssertionError): time.sleep(1)
        raise RuntimeError("Synthetic app did not become healthy")
    def login():
        return request("/api/v1/auth/login", "POST", {"email": "admin@example.invalid", "password": password})[0]["token"]
    def me(token, tenant=None):
        return request("/api/v1/auth/me", token=token, tenant=tenant)[0]["data"]
    def oauth(subject):
        callback = origin + "/api/v1/auth/oidc/callback"
        auth = request("/api/v1/auth/oidc/url?" + urllib.parse.urlencode({"redirect_uri": callback}))[0]
        state = urllib.parse.parse_qs(urllib.parse.urlsplit(auth["authorization_url"]).query)["state"][0]
        _, headers, _ = request("/api/v1/auth/oidc/callback?" + urllib.parse.urlencode({"state": state, "code": subject}), expected=(302,))
        fragment = urllib.parse.parse_qs(urllib.parse.urlsplit(headers["Location"]).fragment)
        if "oidc_result" not in fragment: raise AssertionError("Synthetic OAuth callback failed: " + str(fragment.get("oidc_error_description", fragment.get("oidc_error"))))
        encoded = fragment["oidc_result"][0]
        return json.loads(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))
    try:
        docker("up", "-d", "--pull", "never")
        wait_app()
        request("/api/v1/auth/register", "POST", {"username": "admin", "email": "admin@example.invalid", "password": password}, expected=(201,))
        admin = login()
        before = me(admin)
        check("admin.register_tenantless", before["tenant"] is None and not before["user"].get("is_system_admin"))
        request("/api/v1/auth/register", "POST", {"username": "admin", "email": "admin@example.invalid", "password": password}, expected=(400,409))
        check("admin.duplicate_registration_rejected")
        docker("restart", "app")
        wait_app()
        admin = login()
        info = me(admin)
        admin_id = info["user"]["id"]
        check("admin.bootstrap_promotes_existing_user", info["user"]["is_system_admin"] and not info["user"].get("can_access_all_tenants"))
        tenant = request("/api/v1/tenants", "POST", {"name": "R1 Company", "description": "Synthetic R1 only"}, token=admin, expected=(201,))[0]["data"]["id"]
        def members(token=None):
            data = request(f"/api/v1/tenants/{tenant}/members", token=token or admin, tenant=tenant)[0]["data"]
            return data if isinstance(data, list) else data["members"]
        def role(uid): return next((m["role"] for m in members() if m["user_id"] == uid), None)
        check("admin.creates_default_space_as_owner", role(admin_id) == "owner")
        version = request("/api/v1/system/info", token=admin, tenant=tenant)[0]["data"]
        check("upstream.version", version["version"].lstrip("v") == "0.8.0", version=version["version"], commit_id=version.get("commit_id", ""))
        request("/api/v1/system/admin/settings/auth.registration_mode", "PUT", {"value": "invite_only"}, token=admin, tenant=tenant)
        request("/api/v1/auth/register", "POST", {"username": "blocked", "email": "blocked@example.invalid", "password": password}, expected=(403,))
        check("admin.closes_public_registration")
        request("/api/v1/system/admin/settings/tenant.self_service_creation_enabled", "PUT", {"value": False}, token=admin, tenant=tenant)
        request("/api/v1/tenants", "POST", {"name": "R1 Must Not Exist"}, token=admin, tenant=tenant, expected=(403,))
        check("workspace.system_admin_has_no_native_creation_exemption")
        request("/api/v1/system/admin/settings/tenant.self_service_creation_enabled", "PUT", {"value": True}, token=admin, tenant=tenant)
        if args.through == "bootstrap":
            report["status"] = "passed"
            return
        employee = oauth("r1-employee")
        emp_token, emp_id = employee["token"], employee["user"]["id"]
        check("employee.oauth_provisions_without_personal_space", me(emp_token)["tenant"] is None)
        repeat = oauth("r1-employee")
        check("employee.repeated_oauth_reuses_account", repeat["user"]["id"] == emp_id)
        key = request(f"/api/v1/tenants/{tenant}/api-keys", "POST", {"name": "R1 onboarding", "full_access": False, "capabilities": ["manage_members"]}, token=admin, tenant=tenant, expected=(201,))[0]["data"]["token"]
        key_headers = {"X-API-Key": key}
        request(f"/api/v1/tenants/{tenant}/members", "POST", {"email": "r1-employee@example.invalid", "role": "viewer"}, headers=key_headers, tenant=tenant, expected=(201,))
        check("employee.scoped_key_adds_viewer", role(emp_id) == "viewer" and me(emp_token, tenant)["tenant"]["id"] == tenant)
        request(f"/api/v1/tenants/{tenant}/members", "POST", {"email": "r1-employee@example.invalid", "role": "viewer"}, headers=key_headers, tenant=tenant, expected=(409,))
        check("employee.duplicate_membership_conflicts_without_duplicate_row", len([m for m in members() if m["user_id"] == emp_id]) == 1)
        request(f"/api/v1/tenants/{tenant}/members/{emp_id}", "PUT", {"role": "owner"}, headers=key_headers, tenant=tenant, expected=(403,))
        check("member.api_key_cannot_assign_owner")
        request(f"/api/v1/tenants/{tenant}/members/{admin_id}", "PUT", {"role": "admin"}, token=admin, tenant=tenant, expected=(400,409))
        check("member.last_owner_cannot_be_demoted")
        for r in ("admin", "contributor", "viewer"):
            request(f"/api/v1/tenants/{tenant}/members/{emp_id}", "PUT", {"role": r}, token=admin, tenant=tenant)
            check("member.native_role_" + r, role(emp_id) == r)
            request(f"/api/v1/tenants/{tenant}/members/{admin_id}", "PUT", {"role": "viewer"}, token=emp_token, tenant=tenant, expected=(403,))
            if r == "contributor":
                owned_agent = request("/api/v1/agents", "POST", {"name": "R1 Contributor Agent"}, token=emp_token, tenant=tenant, expected=(201,))[0]["data"]["id"]
                request(f"/api/v1/agents/{owned_agent}", "PUT", {"name": "R1 Contributor Updated"}, token=emp_token, tenant=tenant)
                check("member.contributor_creates_and_updates_own_agent")
            if r == "viewer":
                request("/api/v1/agents", "POST", {"name": "R1 Denied Viewer Agent"}, token=emp_token, tenant=tenant, expected=(403,))
                request(f"/api/v1/agents/{owned_agent}", "PUT", {"name": "R1 Native Creator Exception"}, token=emp_token, tenant=tenant)
                check("member.viewer_creation_denied_native_creator_edit_preserved")
        request("/api/v1/tenants", "POST", {"name": "R1 Native Self Service"}, token=emp_token, tenant=tenant, expected=(201,))
        check("workspace.native_self_service_allows_viewer_requires_gateway_policy")
        agent = request("/api/v1/agents", "POST", {"name": "R1 Assistant", "config": {"knowledge_bases": []}}, token=admin, tenant=tenant, expected=(201,))[0]["data"]["id"]
        channel = request(f"/api/v1/agents/{agent}/embed-channels", "POST", {"name": "R1 Embed", "allowed_origins": ["https://r1.example.invalid"], "enabled": True}, token=admin, tenant=tenant, expected=(201,))[0]["data"]
        channel_id = channel["id"]
        embed_headers = {"Authorization": "Embed " + channel["publish_token"], "Origin": "https://r1.example.invalid"}
        issued = request(f"/api/v1/embed/{channel_id}/exchange", "POST", {}, headers=embed_headers)[0]["data"]
        embed_headers["Authorization"] = "Embed " + issued["session_token"]
        session = request(f"/api/v1/embed/{channel_id}/sessions", "POST", {}, headers=embed_headers, expected=(201,))[0]["data"]
        check("embed.channel_token_creates_session_without_employee", bool(session["id"] and session["sig"]))
        request(f"/api/v1/embed/{channel_id}/config", headers={**embed_headers, "Origin": "https://wrong.example.invalid"}, expected=(403,))
        check("embed.origin_enforced")
        request(f"/api/v1/tenants/{tenant}/members/{emp_id}", "DELETE", token=admin, tenant=tenant)
        request("/api/v1/auth/me", token=emp_token, tenant=tenant, expected=(401,403))
        emp_token = oauth("r1-employee")["token"]
        request("/api/v1/knowledge-bases", token=emp_token, tenant=tenant, expected=(403,))
        check("employee.removed_member_not_readded_by_native_login", role(emp_id) is None)
        request(f"/api/v1/embed/{channel_id}/config", headers=embed_headers)
        check("embed.channel_token_survives_employee_removal_requires_adapter")
        request(f"/api/v1/embed-channels/{channel_id}", "PUT", {"enabled": False, "allowed_origins": ["https://r1.example.invalid"]}, token=admin, tenant=tenant)
        request(f"/api/v1/embed/{channel_id}/config", headers=embed_headers, expected=(403,))
        check("embed.channel_disable_revokes_token")
        invite = request(f"/api/v1/tenants/{tenant}/invitations", "POST", {"email": "r1-employee@example.invalid", "role": "contributor"}, token=admin, tenant=tenant, expected=(201,))[0]["data"]
        request(f"/api/v1/me/invitations/{invite['id']}/accept", "POST", {}, token=emp_token)
        check("member.invitation_accept_preserves_contributor", role(emp_id) == "contributor")
        request(f"/api/v1/tenants/{tenant}/members/{emp_id}", "PUT", {"role": "owner"}, token=admin, tenant=tenant)
        request(f"/api/v1/tenants/{tenant}/members/{admin_id}", "PUT", {"role": "admin"}, token=emp_token, tenant=tenant)
        check("member.ownership_transfer", role(emp_id) == "owner" and role(admin_id) == "admin")
        docker("restart", "app")
        wait_app()
        admin = login()
        check("admin.restart_does_not_reclaim_transferred_ownership", role(admin_id) == "admin" and me(admin, tenant)["user"]["is_system_admin"])
        report["status"] = "passed"
    except Exception as error:
        report["status"] = "failed"
        report["failure"] = str(error)
        try: (run / "app.log").write_text(docker("logs", "--no-color", "--tail", "120", "app"))
        except Exception: pass
        raise
    finally:
        try:
            docker("down", "--volumes", "--remove-orphans")
            report["cleanup"] = "only disposable project removed"
        finally:
            compose.unlink(missing_ok=True)
            output = ROOT / "docs/plans/evidence" / ("r1-api-bootstrap.json" if args.through == "bootstrap" else "r1-api-probe.json")
            output.parent.mkdir(parents=True, exist_ok=True)
            report["probe_sha256"] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
            output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
            print("Report: " + str(output), flush=True)


if __name__ == "__main__":
    main()
