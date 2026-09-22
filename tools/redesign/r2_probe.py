#!/usr/bin/env python3
"""Disposable, real-upstream HTTP probes for R2; never use an existing stack.

Creates its own randomly named Docker Compose project using locally cached images,
an internal network (R5 additionally publishes loopback browser ports).
Databases and files are synthetic.
Only this project's containers/volumes are removed in finally. No source patches,
company OAuth, model endpoints, existing runtime env files or existing volumes.
"""
from __future__ import annotations

import base64
from concurrent.futures import ThreadPoolExecutor
import argparse
import datetime
import hashlib
import http.cookies
import http.client
import json
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.parse

from r2_evidence import source_digest

ROOT = Path(__file__).resolve().parents[2]
IMAGES = {"app": "wechatopenai/weknora-app:v0.8.0", "postgres": "paradedb/paradedb:v0.22.2-pg17", "redis": "redis:7-alpine", "identity": "python:3.12-alpine"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--through", choices=("bootstrap", "all"), default="all")
    parser.add_argument("--profile", choices=("r2", "r3", "r4", "r5", "graph"), default="r2", help="R3 reuses this isolated R2 regression harness and writes separate evidence")
    args = parser.parse_args()
    project = "mindcreek-" + args.profile + "-" + secrets.token_hex(4)
    local = ROOT / (".local/redesign-" + args.profile)
    local.mkdir(parents=True, exist_ok=True)
    run = Path(tempfile.mkdtemp(prefix=project + "-", dir=local))
    compose = run / "compose.json"
    base = ["docker", "compose", "--project-name", project, "-f", str(compose)]
    report = {"time": datetime.datetime.now(datetime.timezone.utc).isoformat(), "project": project, "scope": "R2 gateway, native v0.8.0, synthetic corporate OAuth broker flow and disposable PostgreSQL; no real enterprise/model acceptance", "checks": [], "requests": [], "images": {}}
    def docker(*args):
        result = subprocess.run([*base, *args], text=True, capture_output=True)
        if result.returncode:
            (run / "docker-error.log").write_text(result.stderr)
            raise RuntimeError("Docker command failed; local diagnostics: " + str(run / "docker-error.log"))
        if report['project'].startswith('mindcreek-r5-') and args[:2] == ('restart','gateway'):
            # Compose restart waits for the process, not application readiness.
            # Cover restarts in inherited R3 and R5 helpers with the same barrier.
            wait_app()
        return result.stdout.strip()
    if args.profile == "graph":
        IMAGES["redis"] = "redis:7.0-alpine"
    for key, name in IMAGES.items():
        # Pin the cached immutable ID; do not pull a moving tag during the probe.
        selected = name
        if args.profile == "r5":
            # The release workflow loaded AMD64 variants under the same tags.
            # Reuse the cached native-host variants from R4's immutable record;
            # BM25 under AMD64 emulation is a separately recorded R6 limitation.
            selected = json.loads((ROOT/'docs/plans/evidence/r4-api-probe.json').read_text())['images'][key]['id']
        report["images"][key] = {"tag": name, "id": subprocess.check_output(["docker", "image", "inspect", selected, "--format", "{{.Id}}"], text=True).strip()}
    secret = secrets.token_hex(16)
    password = "R2-" + secrets.token_hex(10) + "!"
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
        "postgres": {"image": report["images"]["postgres"]["id"], "environment": {"POSTGRES_USER": "r1", "POSTGRES_PASSWORD": secret, "POSTGRES_DB": "r1"}, "tmpfs": ["/var/lib/postgresql/data"], "healthcheck": {"test": ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U r1 -d r1"], "interval": "2s", "timeout": "2s", "retries": 30}},
        "redis": {"image": report["images"]["redis"]["id"], "command": ["redis-server", "--save", "", "--appendonly", "no"], "tmpfs": ["/data"]},
        "identity": {"image": report["images"]["identity"]["id"], "command": ["python", "/fixture/mock_identity.py"], "volumes": [str(ROOT / "testdata/redesign/mock_identity.py") + ":/fixture/mock_identity.py:ro"]},
        "app": {"image": report["images"]["app"]["id"], "environment": environment, "tmpfs": ["/data/files"], "depends_on": {"postgres": {"condition": "service_healthy"}, "redis": {"condition": "service_started"}, "identity": {"condition": "service_started"}}},
    }
    binary = local / "gateway-linux"
    if not binary.is_file(): raise RuntimeError("Run make r2-gateway-build first")
    build = json.loads((local / "gateway-build.json").read_text())
    if build["gateway_sha256"] != hashlib.sha256(binary.read_bytes()).hexdigest() or build["gateway_source_sha256"] != source_digest():
        raise RuntimeError("Gateway source/build changed; run make r2-gateway-build")
    report.update(build)
    secret_dir = run / "secrets"
    secret_dir.mkdir(mode=0o700)
    (secret_dir / "admin-password").write_text(password)
    (secret_dir / "admin-password").chmod(0o600)
    environment.update({
        "OIDC_AUTH_ISSUER_URL": "http://gateway:8080/api/v1/mindcreek/oidc",
        "OIDC_AUTH_CLIENT_ID": "mindcreek-weknora", "OIDC_AUTH_CLIENT_SECRET": secret,
        "OIDC_AUTH_AUTHORIZATION_ENDPOINT": "http://gateway:8080/api/v1/mindcreek/oidc/authorize",
        "OIDC_AUTH_TOKEN_ENDPOINT": "http://gateway:8080/api/v1/mindcreek/oidc/token",
        "OIDC_AUTH_USER_INFO_ENDPOINT": "http://gateway:8080/api/v1/mindcreek/oidc/userinfo",
        "OIDC_USER_INFO_MAPPING_USER_NAME": "preferred_username", "SSRF_WHITELIST_EXTRA": "identity,gateway",
    })
    gateway_env = {
        "MINDCREEK_DATABASE_URL": f"postgres://r1:{secret}@postgres:5432/r1?sslmode=disable",
        "MINDCREEK_UPSTREAM_URL": "http://app:8080", "MINDCREEK_ENTERPRISE_ENABLED": "true",
        "MINDCREEK_ROUTE_POLICY_FILE": "/config/phase1-route-policy.json", "MINDCREEK_ROUTE_ACTIONS_FILE": "/config/phase2-route-actions.json",
        "MINDCREEK_CAPABILITIES_FILE": "/config/phase5-capabilities.json",
        "MINDCREEK_INSTALL_ADMIN_EMAIL": "admin@example.invalid", "MINDCREEK_INSTALL_ADMIN_USERNAME": "admin",
        "MINDCREEK_INSTALL_ADMIN_PASSWORD_FILE": "/secrets/admin-password", "MINDCREEK_DEFAULT_SPACE_NAME": "R2 Company",
        "MINDCREEK_MEMBER_KEY_FILE": "/secrets/member-key",
        "MINDCREEK_RECOVERY_OWNER_BEARER_FILE": "/secrets/recovery-owner-bearer",
        "MINDCREEK_IDENTITY_ENABLED": "true", "MINDCREEK_IDENTITY_PROTOCOL": "oauth2",
        "MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP": "true", "MINDCREEK_EXTERNAL_ORIGIN": "http://gateway:8080",
        "MINDCREEK_IDENTITY_ISSUER": "http://identity:18000", "MINDCREEK_IDENTITY_CLIENT_ID": "r2-corporate",
        "MINDCREEK_IDENTITY_CLIENT_SECRET": secret, "MINDCREEK_BROKER_CLIENT_SECRET": secret,
        "MINDCREEK_IDENTITY_AUTHORIZATION_URL": "http://identity:18000/authorize",
        "MINDCREEK_IDENTITY_TOKEN_URL": "http://identity:18000/token", "MINDCREEK_IDENTITY_USERINFO_URL": "http://identity:18000/userinfo",
        "MINDCREEK_IDENTITY_TOKEN_REQUEST_FORMAT": "form", "MINDCREEK_IDENTITY_AUTHORIZATION_METHOD": "GET",
        "MINDCREEK_IDENTITY_SUBJECT_CLAIM": "sub", "MINDCREEK_IDENTITY_USERNAME_CLAIM": "name",
        "MINDCREEK_IDENTITY_DISPLAY_NAME_CLAIM": "name", "MINDCREEK_IDENTITY_EMAIL_CLAIM": "email",
        "MINDCREEK_IDENTITY_SUBJECT_TENANT_SCOPED": "false", "MINDCREEK_IDENTITY_STATE_REQUIRED": "true",
    }
    if args.profile in ("r3", "r4", "r5", "graph"):
        from r3_probe import configure
        configure(ROOT, run, services, environment, gateway_env, report)
    if args.profile == "r4":
        from r4_evidence import inputs_digest
        report["r4_inputs_sha256"] = inputs_digest()
    services["gateway"] = {"image": report["images"]["identity"]["id"], "entrypoint": ["/r2/gateway"],
        "environment": gateway_env, "volumes": [str(binary)+":/r2/gateway:ro",str(secret_dir)+":/secrets",str(ROOT/"config")+":/config:ro"],
        "depends_on": {"postgres": {"condition": "service_healthy"},"app": {"condition": "service_started"}}}
    if args.profile in ("r3", "r4", "r5", "graph"):
        # PostgreSQL's image briefly starts a bootstrap-only server; native
        # TCP may still be unavailable after the first unix-socket health hit.
        services["gateway"]["restart"] = "on-failure:5"
    if args.profile == "r5":
        from r5_live import configure
        configure(ROOT, run, services, environment, gateway_env, report)
    if args.profile == "graph":
        import sys
        sys.path.insert(0, str(ROOT / "tools/graph"))
        from graph_probe import configure_graph
        configure_graph(ROOT, run, services, report)
    networks={"default": {"internal": True}}
    if args.profile == "r5": networks["browser"]={}
    compose.write_text(json.dumps({"services": services, "networks": networks, "volumes": {"mindcreek_graph_data": {}} if args.profile == "graph" else {}}))
    compose.chmod(0o600)
    origin = "http://gateway:8080"
    cookies = {}
    transport = '''import base64,json,sys,urllib.request,urllib.error
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*a): return None
p=json.load(sys.stdin)
data=base64.b64decode(p['raw_body']) if p.get('raw_body') is not None else None if p['body'] is None else json.dumps(p['body']).encode()
req=urllib.request.Request(p['url'],data=data,method=p['method'],headers=p['headers'])
try:
 try: r=urllib.request.build_opener(urllib.request.ProxyHandler({}),NoRedirect()).open(req,timeout=12)
 except urllib.error.HTTPError as e: r=e
 raw=r.read()
 print(json.dumps({'status':r.code,'headers':dict(r.headers),'body':json.loads(raw) if raw.startswith((b'{',b'[')) else {}}))
except OSError:
 print(json.dumps({'connection_error':True}))
'''
    def request(path, method="GET", body=None, token=None, tenant=None, headers=None, expected=(200,), raw_body=None):
        hdr = {"Content-Type": "application/json", **(headers or {})}
        if cookies: hdr["Cookie"] = "; ".join(k + "=" + v for k, v in cookies.items())
        if token: hdr["Authorization"] = "Bearer " + token
        if tenant: hdr["X-Tenant-ID"] = str(tenant)
        if args.profile == "r5" and path.startswith("http://mindcreek.localhost:18685/"):
            path = "http://gateway:8080" + path.removeprefix("http://mindcreek.localhost:18685")
        if args.profile == "r5" and path != "/health" and (not path.startswith("http://") or path.startswith(origin + "/")):
            # R5 publishes only a disposable loopback nginx. Real HTTP here
            # avoids a Docker exec process per request; credentials unchanged.
            conn=http.client.HTTPConnection('127.0.0.1',18685,timeout=90)
            try:
                conn.request(method,path.removeprefix(origin),body=raw_body if raw_body is not None else None if body is None else json.dumps(body).encode(),headers={**hdr,'Host':'mindcreek.localhost:18685'})
                resp=conn.getresponse(); raw=resp.read()
                response={'status':resp.status,'headers':dict(resp.getheaders()),'body':json.loads(raw) if raw.startswith((b'{',b'[')) else {}}
            finally:conn.close()
        else:
            result = subprocess.run([*base, "exec", "-T", "identity", "python", "-c", transport], input=json.dumps({"url": path if path.startswith("http://") else origin + path, "method": method, "body": body, "raw_body":None if raw_body is None else base64.b64encode(raw_body).decode(), "headers": hdr}), capture_output=True, text=True, check=True)
            response = json.loads(result.stdout)
        if response.get("connection_error"): raise OSError("app not ready")
        data, status, response_headers = response["body"], response["status"], response["headers"]
        if path != "/health":
            auth = "embed" if hdr.get("Authorization", "").startswith("Embed ") else "bearer" if token else "member_api_key" if hdr.get("X-API-Key") else "none"
            report["requests"].append({"method": method, "path": path.split("?")[0], "auth": auth, "workspace_header": bool(tenant), "expected_status": list(expected), "actual_status": status})
        if args.profile in ("r3", "r4", "r5", "graph") and path == "http://models:19090/counts":
            report["requests"][-1]["execution_counts"] = data
        parsed = http.cookies.SimpleCookie(response_headers.get("Set-Cookie", ""))
        for key, value in parsed.items(): cookies[key] = value.value
        if status not in expected:
            # Never include successful auth/key payloads in logs or reports.
            raise AssertionError(f"{method} {path.split('?')[0]}: status {status}; expected {expected}; error {str(data.get('error', data.get('message', '')))[:240]}")
        return data, response_headers, status
    def check(name, condition=True, **facts):
        if not condition: raise AssertionError(name)
        if args.profile == 'r5':
            # The newest populated migration (000016) blocks before the older
            # R2/R3 guards. Keep legacy evidence names out of this new report.
            name={'r3.migration.populated_audit_refuses_rollback':'r5.migration.channel_bindings_refuse_rollback',
                  'migration.rejects_erasing_completed_onboarding':'r5.migration.channel_bindings_still_guard_after_restart'}.get(name,name)
        report["checks"].append({"name": name, "passed": True, **facts})
        print("PASS " + name, flush=True)
    def wait_app(target=None):
        # /health on the R5 frontend is the SPA fallback, not gateway readiness.
        # request() deliberately sends this probe to the actual internal service.
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            try:
                request(target + "/health" if target else "/health")
                return
            except (OSError, AssertionError): time.sleep(1)
        raise RuntimeError("Synthetic app did not become healthy")
    def login():
        return request("/api/v1/mindcreek/admin/auth/login", "POST", {"email": "admin@example.invalid", "password": password})[0]["token"]
    def me(token, tenant=None):
        return request("/api/v1/auth/me", token=token, tenant=tenant)[0]["data"]
    def oauth(subject):
        # Native JWT iat/revocation timestamps have one-second granularity.
        # Separate intentional revoke/relogin probes without weakening checks.
        if args.profile == "r5": time.sleep(1.05)
        callback = ("http://mindcreek.localhost:18685" if args.profile == "r5" else origin) + "/api/v1/auth/oidc/callback"
        auth = request("/api/v1/auth/oidc/url?" + urllib.parse.urlencode({"redirect_uri": callback}))[0]
        _, hdr, _ = request(auth["authorization_url"], expected=(302,))
        state = urllib.parse.parse_qs(urllib.parse.urlsplit(hdr["Location"]).query)["state"][0]
        _, hdr, _ = request("/api/v1/mindcreek/oidc/callback?" + urllib.parse.urlencode({"state":state,"code":subject}),expected=(302,))
        _, hdr, _ = request(hdr["Location"],expected=(302,))
        fragment = urllib.parse.parse_qs(urllib.parse.urlsplit(hdr["Location"]).fragment)
        if "oidc_result" not in fragment: raise AssertionError("Synthetic corporate OAuth callback failed")
        encoded=fragment["oidc_result"][0]
        return json.loads(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))
    def cli(*args, expected=0):
        p=subprocess.run([*base,"exec","-T","gateway","/r2/gateway",*args],text=True,capture_output=True)
        if p.returncode!=expected:
            (run/"cli-error.log").write_text(p.stderr)
            raise AssertionError("Installer command failed: "+" ".join(args))
        if args == ('migrate','down','1') and expected == 1 and report['project'].startswith('mindcreek-r5-'):
            if 'Refusing to remove populated employee channel session bindings' not in p.stderr:
                raise AssertionError('Expected migration 000016 channel-binding protection')
        return json.loads(p.stdout) if p.returncode==0 and p.stdout.strip().startswith("{") else None
    try:
        docker("up", "-d", "--pull", "never")
        wait_app()
        wait_app("http://app:8080")
        cli("migrate","down","4" if args.profile == "r5" else "3" if args.profile in ("r3", "r4", "graph") else "2")
        cli("migrate","up")
        check("migration.empty_rollback_forward")
        first=cli("install","prepare")
        check("install.tenantless_admin_created",first["stage"]=="admin_registered")
        again=cli("install","prepare")
        check("install.repeat_prepare_same_account",again["admin_user_id"]==first["admin_user_id"])
        request("/api/v1/mindcreek/admin/auth/login","POST",{"email":"admin@example.invalid","password":password},expected=(401,))
        check("admin.rejects_before_native_promotion")
        environment["DISABLE_REGISTRATION"]="true"
        compose.write_text(json.dumps({"services": services, "networks": networks, "volumes": {"mindcreek_graph_data": {}} if args.profile == "graph" else {}}))
        docker("up","-d","--no-deps","--force-recreate","app")
        time.sleep(2)
        # Account confirmation is intentionally separate from workspace creation.
        deadline=time.monotonic()+90
        while True:
            try: account=cli("install","confirm-admin");break
            except AssertionError:
                if time.monotonic()>deadline: raise
                time.sleep(1)
        check("install.native_bootstrap_confirmed",account["stage"]=="account_ready")
        admin=login()
        admin_id=me(admin)["user"]["id"]
        request("/api/v1/mindcreek/installation",token=admin)
        request("/api/v1/knowledge-bases",token=admin,expected=(409,))
        check("admin.tenantless_setup_only")
        with ThreadPoolExecutor(max_workers=2) as pool:
            installs=list(pool.map(lambda _: cli("install","default-space"),range(2)))
        installed=installs[0]
        check("install.concurrent_default_space_single_id",all(x["default_tenant_id"]==installed["default_tenant_id"] for x in installs))
        tenant=installed["default_tenant_id"]
        check("install.default_space_and_scoped_key",installed["stage"]=="ready" and bool(installed["member_key_id"]))
        admin=login()
        def members(token= None,space=None):
            return request(f"/api/v1/tenants/{space or tenant}/members",token=token or admin,tenant=space or tenant)[0]["data"]["members"]
        def role(uid): return next((m["role"] for m in members() if m["user_id"]==uid),None)
        check("install.admin_is_default_owner",role(admin_id)=="owner")
        version=request("/api/v1/system/info",token=admin,tenant=tenant)[0]["data"]
        check("upstream.version",version["version"].lstrip("v")=="0.8.0",version=version["version"],commit_id=version.get("commit_id"))
        key=(secret_dir/"member-key").read_text()
        request("/api/v1/tenants","POST",{"name":"forbidden-machine"},headers={"X-API-Key":key},expected=(403,))
        request("/api/v1/auth/register","POST",{},expected=(404,))
        request("/api/v1/auth/login","POST",{},expected=(404,))
        check("security.employee_password_and_machine_space_creation_closed")
        if args.through=="bootstrap":
            report["status"]="passed"
            return
        cli("migrate","down","4" if args.profile == "r5" else "3" if args.profile in ("r3", "r4", "graph") else "2",expected=1)
        cli("migrate","up")
        check("migration.rejects_erasing_installation_and_restores_schema")
        employee=oauth("r2-employee")
        employee_id=employee["user"]["id"];emp=employee["token"]
        check("employee.corporate_oauth_without_personal_space",me(emp)["tenant"] is None)
        check("employee.identity_only_status",request("/api/v1/mindcreek/onboarding",token=emp)[0]["data"]["state"]=="pending")
        for path in ("/api/v1/knowledge-bases","/api/v1/agents","/api/v1/sessions","/mcp"):
            request(path,token=emp,expected=(409,))
        request("/api/v1/mindcreek/onboarding","POST",{"role":"owner"},token=emp,expected=(400,))
        check("employee.pending_knowledge_denied_and_role_input_rejected")
        with ThreadPoolExecutor(max_workers=4) as pool:
            attempts=list(pool.map(lambda _: request("/api/v1/mindcreek/onboarding","POST",{},token=emp)[0]["data"],range(4)))
        joined=attempts[0]
        check("employee.concurrent_postgres_onboarding",all(x["state"]=="ready" for x in attempts) and sum(m["user_id"]==employee_id for m in members())==1)
        check("employee.auto_join_viewer",joined["state"]=="ready" and role(employee_id)=="viewer")
        request("/api/v1/mindcreek/onboarding","POST",{},token=emp)
        repeated=oauth("r2-employee");emp=repeated["token"]
        check("employee.repeat_oauth_reuses_account",repeated["user"]["id"]==employee_id and role(employee_id)=="viewer")
        check("employee.no_create_capability",not me(emp)["capabilities"]["can_create_tenant"])
        request("/api/v1/tenants","POST",{"name":"forbidden-employee"},token=emp,tenant=tenant,expected=(403,))
        # An employee refresh token cannot become a local admin session.
        request("/api/v1/mindcreek/admin/auth/refresh","POST",{"refreshToken":repeated["refresh_token"]},expected=(401,))
        emp=oauth("r2-employee")["token"]
        check("admin.refresh_rejects_employee")
        for assigned in ("contributor","admin","viewer"):
            request(f"/api/v1/tenants/{tenant}/members/{employee_id}","PUT",{"role":assigned},token=admin,tenant=tenant)
            request(f"/api/v1/tenants/{tenant}/invitations","POST",{"email":"r2-employee@example.invalid","role":"viewer"},token=emp,tenant=tenant,expected=(403,))
            check("member.native_role_"+assigned,role(employee_id)==assigned)
        request(f"/api/v1/tenants/{tenant}/members/{employee_id}","PUT",{"role":"contributor"},token=admin,tenant=tenant)
        request("/api/v1/mindcreek/onboarding","POST",{},token=emp)
        check("employee.relogin_preserves_contributor",role(employee_id)=="contributor")
        request(f"/api/v1/tenants/{tenant}/members/{admin_id}","PUT",{"role":"admin"},token=admin,tenant=tenant,expected=(409,))
        check("member.last_owner_protected")
        other=request("/api/v1/tenants","POST",{"name":"R2 Other"},token=admin,tenant=tenant,expected=(201,))[0]["data"]["id"]
        check("workspace.platform_admin_creates_as_owner",any(m["user_id"]==admin_id and m["role"]=="owner" for m in members(space=other)))
        invitation=request(f"/api/v1/tenants/{other}/invitations","POST",{"email":"r2-employee@example.invalid","role":"contributor"},token=admin,tenant=other,expected=(201,))[0]["data"]
        request(f"/api/v1/me/invitations/{invitation['id']}/accept","POST",{},token=emp)
        check("member.corporate_email_invitation_accept",any(m["tenant_id"]==other and m["role"]=="contributor" for m in me(emp)["memberships"]))
        request(f"/api/v1/tenants/{other}/members","POST",{"email":"not-registered@example.invalid","role":"viewer"},token=admin,tenant=other,expected=(409,))
        check("member.unregistered_email_not_provisioned")
        request(f"/api/v1/tenants/{other}/members/{admin_id}","PUT",{"role":"viewer"},token=emp,tenant=tenant,expected=(403,))
        check("member.cross_space_tampering_denied")
        for setting,value in (("auth.default_tenant_mode","create_personal"),("auth.registration_mode","self_serve"),("tenant.self_service_creation_enabled",False)):
            request("/api/v1/system/admin/settings/"+setting,"PUT",{"value":value},token=admin,tenant=tenant,expected=(409,))
        check("settings.enterprise_contract_protected")
        revoked=oauth("r2-removed");removed_id=revoked["user"]["id"]
        request("/api/v1/mindcreek/onboarding","POST",{},token=revoked["token"])
        request(f"/api/v1/tenants/{tenant}/members/{removed_id}","DELETE",token=admin,tenant=tenant)
        revoked=oauth("r2-removed")
        status=request("/api/v1/mindcreek/onboarding","POST",{},token=revoked["token"])[0]["data"]
        check("employee.removed_not_readded",status["state"]=="removed" and not status["memberships"] and role(removed_id) is None)
        request(f"/api/v1/tenants/{other}/members","POST",{"email":"r2-removed@example.invalid","role":"viewer"},token=admin,tenant=other,expected=(201,))
        status=request("/api/v1/mindcreek/onboarding",token=revoked["token"])[0]["data"]
        check("employee.removed_can_keep_other_workspace",status["state"]=="removed" and len(status["memberships"])==1)
        request("http://app:8080"+f"/api/v1/tenants/{tenant}/members/{employee_id}","PUT",{"role":"owner"},headers={"X-API-Key":key},tenant=tenant,expected=(403,))
        request("http://app:8080"+f"/api/v1/tenants/{other}/members",headers={"X-API-Key":key},tenant=other,expected=(403,))
        check("install.service_key_cannot_grant_owner_or_cross_space")
        if args.profile == "r5":
            from r5_live import assistant_checks
            assistant_checks(ROOT, run, request, check, oauth, docker, tenant, other, admin, password, base)
        if args.profile == "r4":
            from r4_live import browser_checks
            admin = browser_checks(ROOT, run, request, check, oauth, docker, tenant, other, admin, password, base)
        if args.profile == "graph":
            from graph_probe import graph_checks
            graph_checks(ROOT, run, request, check, oauth, docker, tenant, other, admin, emp, employee_id, base)
        if args.profile in ("r3", "r4", "r5"):
            from r3_probe import resource_checks
            emp = resource_checks(request, check, oauth, cli, docker, tenant, other, admin, emp, employee_id, run)
        request(f"/api/v1/tenants/{tenant}/members/{employee_id}","PUT",{"role":"owner"},token=admin,tenant=tenant)
        request(f"/api/v1/tenants/{tenant}/members/{admin_id}","PUT",{"role":"admin"},token=admin,tenant=tenant)
        check("member.ownership_transferred_platform_identity_retained",role(employee_id)=="owner" and role(admin_id)=="admin" and me(admin)["user"]["is_system_admin"])
        request("/api/v1/tenants","POST",{"name":"forbidden-owner"},token=emp,tenant=tenant,expected=(403,))
        check("workspace.transferred_owner_has_no_platform_creation")
        cli("install","prepare");cli("install","confirm-admin");cli("install","default-space")
        docker("restart","gateway");wait_app()
        check("install.restart_does_not_reclaim_owner",role(admin_id)=="admin")
        cli("migrate","down","1",expected=1)
        check("migration.rejects_erasing_completed_onboarding")
        request(f"/api/v1/tenants/{tenant}/api-keys/{installed['member_key_id']}","DELETE",token=emp,tenant=tenant)
        blocked=oauth("r2-key-failure")
        state=request("/api/v1/mindcreek/onboarding","POST",{},token=blocked["token"])[0]["data"]
        check("employee.revoked_service_key_fails_closed",state["state"]=="failed" and me(blocked["token"])["tenant"] is None)
        replacement=request(f"/api/v1/tenants/{tenant}/api-keys","POST",{"name":"replacement onboarding","full_access":False,"capabilities":["manage_members"]},token=emp,tenant=tenant,expected=(201,))[0]["data"]
        (secret_dir/"member-key").write_text(replacement["token"])
        (secret_dir/"recovery-owner-bearer").write_text(emp)
        (secret_dir/"recovery-owner-bearer").chmod(0o600)
        cli("install","repair-member-key","--member-key-id",str(replacement["id"]))
        recovered=request("/api/v1/mindcreek/onboarding","POST",{},token=blocked["token"])[0]["data"]
        check("install.current_owner_repairs_revoked_key",recovered["state"]=="ready" and role(admin_id)=="admin")
        credentials=request("/api/v1/mindcreek/admin/auth/login","POST",{"email":"admin@example.invalid","password":password})[0]
        refresh=request("/api/v1/mindcreek/admin/auth/refresh","POST",{"refreshToken":credentials["refresh_token"]})[0]
        admin=refresh["access_token"]
        check("admin.native_refresh_dto",bool(refresh["refresh_token"]))
        new_password="R2-New-"+secrets.token_hex(12)+"!"
        request("/api/v1/mindcreek/admin/auth/change-password","POST",{"old_password":password,"new_password":new_password},token=admin)
        request("/api/v1/auth/me",token=admin,expected=(401,))
        request("/api/v1/mindcreek/admin/auth/refresh","POST",{"refreshToken":refresh["refresh_token"]},expected=(401,))
        check("admin.password_rotation_revokes_access_and_refresh")
        session=request("/api/v1/mindcreek/admin/auth/login","POST",{"email":"admin@example.invalid","password":new_password})[0]
        request("/api/v1/mindcreek/admin/auth/logout","POST",{},token=session["token"])
        request("/api/v1/auth/me",token=session["token"],expected=(401,))
        request("/api/v1/mindcreek/admin/auth/refresh","POST",{"refreshToken":session["refresh_token"]},expected=(401,))
        check("admin.logout_revokes_access_and_refresh")
        report["status"]="passed"
    except Exception as exc:
        report["status"]="failed"
        report["error"] = str(exc)[:300]
        (run/"app-diagnostics.log").write_text(docker("logs","--no-color","app"))
        (run/"gateway-diagnostics.log").write_text(docker("logs","--no-color","gateway"))
        raise
    finally:
        try:
            docker("down","--volumes","--remove-orphans")
            report["cleanup"]="only disposable project removed"
        finally:
            compose.unlink(missing_ok=True)
            for f in secret_dir.iterdir(): f.unlink()
            secret_dir.rmdir()
            report["probe_sha256"]=hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
            target=ROOT/"docs/plans/evidence"/(args.profile + ("-api-bootstrap.json" if args.through=="bootstrap" else "-api-probe.json"))
            target.write_text(json.dumps(report,indent=2)+"\n")

if __name__ == "__main__":
    main()
