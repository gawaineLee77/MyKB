"""Actual browser + native HTTP checks. Only disposable identities/content."""
import json
import os
import subprocess
import threading
import time
from r4_ui_server import create_server


def browser_checks(root, run, request, check, oauth, docker, tenant, other, admin, password, compose_command):
    def call(path, method="GET", body=None, token=None, space=tenant, expected=(200,)):
        return request(path, method, body, token=admin if token is None else token, tenant=space, expected=expected)[0]
    builtin = next(agent for agent in call('/api/v1/agents')['data'] if agent['id'] == 'builtin-quick-answer')
    check('r4.browser_fixture.managed_builtin_agent', builtin['config']['model_id'] == 'builtin-mindcreek-chat')
    actors = {}
    for role in ("owner", "admin", "contributor", "viewer"):
        auth = oauth("r4-" + role)
        token = auth["token"]
        call("/api/v1/mindcreek/onboarding", "POST", {}, token=token)
        user = call("/api/v1/auth/me", token=token)["data"]["user"]
        call(f"/api/v1/tenants/{tenant}/members/{user['id']}", "PUT", {"role":role})
        actors[role] = {"token":token, "id":user["id"], "email":f"r4-{role}@example.invalid"}
    # Registered employee outside the target space, for the browser's batch flow.
    newcomer = oauth("r4-new")
    call("/api/v1/mindcreek/onboarding", "POST", {}, token=newcomer["token"])
    new_id = call("/api/v1/auth/me", token=newcomer["token"])["data"]["user"]["id"]
    call(f"/api/v1/tenants/{tenant}/members/{new_id}", "DELETE")
    removed = oauth('r4-removed')
    call('/api/v1/mindcreek/onboarding', 'POST', {}, token=removed['token'])
    removed_id = call('/api/v1/auth/me', token=removed['token'])['data']['user']['id']
    call(f'/api/v1/tenants/{tenant}/members/{removed_id}', 'DELETE')
    call('/api/v1/auth/me', token=removed['token'], expected=(401,))
    removed = oauth('r4-removed')
    actors['removed'] = {'token':removed['token'], 'id':removed_id}
    check('r4.members.removal_revokes_session_before_relogin')
    preview = call("/api/v1/mindcreek/members/preview", "POST", {"emails":["r4-owner@example.invalid", "r4-new@example.invalid", "not-registered@example.invalid"]})["data"]
    check("r4.members.preview_resolves_roles_without_registration", [row["state"] for row in preview["rows"]] == ["existing", "ready", "unregistered"] and preview["rows"][0]["role"] == "owner")
    for role in ("admin", "contributor", "viewer"):
        call("/api/v1/mindcreek/members/preview", "POST", {"emails":["r4-new@example.invalid"]}, token=actors[role]["token"], expected=(403,))
    check("r4.members.preview_owner_only")
    call('/api/v1/mindcreek/members/preview', 'POST', {'emails':['r4-new@example.invalid'], 'tenant_id':other}, expected=(400,))
    call('/api/v1/mindcreek/members/preview', 'POST', {'emails':['r4-new@example.invalid'] * 501}, expected=(400,))
    check('r4.members.preview_rejects_untrusted_target_and_oversized_batch')
    kb = call("/api/v1/knowledge-bases", "POST", {"name":"员工手册 · 合成验证", "description":"用于 R4 页面验收的合成内容", "type":"document"}, expected=(200,201))["data"]
    manual = call(f"/api/v1/knowledge-bases/{kb['id']}/knowledge/manual", "POST", {"title":"合成操作指南", "content":"# Synthetic R4 guide\n\nThe synthetic recovery code is R4-VERIFY. Restart the test worker and verify its health.", "status":"publish"})["data"]
    for _ in range(75):
        document = call(f"/api/v1/knowledge/{manual['id']}")["data"]
        if document["parse_status"] == "completed": break
        if document["parse_status"] == "failed": raise RuntimeError("Synthetic document failed")
        time.sleep(1)
    check("r4.browser_fixture.native_document_processed", document["parse_status"] == "completed")
    faq = call("/api/v1/knowledge-bases", "POST", {"name":"常见问题 · 合成验证", "type":"faq"}, expected=(200,201))["data"]
    call(f"/api/v1/knowledge-bases/{faq['id']}/faq/entry", "POST", {"standard_question":"合成恢复码是什么？", "answers":["R4-VERIFY"], "is_enabled":True})
    from r4_evidence import ui_digest
    ui_path = (root / ".local/redesign-r4/ui-path").read_text().strip()
    server = create_server(lambda: os.path.join((root / ".local/redesign-r4/ui-path").read_text().strip(), "dist"), compose_command)
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    secrets_file = run / "browser-secrets.json"
    secrets_file.write_text(json.dumps({"actors":actors, "tenant":tenant, "other":other, "password":password, "kb":kb["id"], "faq":faq["id"], "manual":manual["id"]})); secrets_file.chmod(0o600)
    env = os.environ.copy()
    env.update({"R4_BROWSER_SECRETS":str(secrets_file), "R4_UI_ORIGIN":f"http://127.0.0.1:{server.server_port}", "R4_ROOT":str(root)})
    try:
        while True:
            with (root / ".local/redesign-r4/browser.log").open("w") as log:
                result = subprocess.run([env.get("R4_NODE_BIN", "node"), str(root / "tools/redesign/r4_browser.mjs")], env=env, stdout=log, stderr=subprocess.STDOUT)
            # Native logout revokes all sessions of this user, including the
            # fixture bearer. Reauthenticate before further API regression.
            admin = request('/api/v1/mindcreek/admin/auth/login', 'POST',
                            {'email':'admin@example.invalid', 'password':password})[0]['token']
            if not result.returncode: break
            if os.environ.get("R4_BROWSER_ITERATE") != "1": raise RuntimeError("R4 browser checks failed; see .local/redesign-r4/browser.log")
            print("WAIT R4 browser development iteration; separate final clean run required", flush=True)
            retry = root / '.local/redesign-r4/retry-browser'
            deadline = time.monotonic() + 1200
            while not retry.exists():
                if time.monotonic() >= deadline: raise RuntimeError("R4 browser iteration expired")
                time.sleep(1)
            retry.unlink()
            for role, actor in actors.items():
                if role != 'removed': call(f"/api/v1/tenants/{tenant}/members/{actor['id']}", "PUT", {"role":role})
                actor['token'] = oauth('r4-' + role)['token']
            call(f"/api/v1/tenants/{tenant}/members/{new_id}", "DELETE", expected=(200,404))
            secrets_file.write_text(json.dumps({"actors":actors, "tenant":tenant, "other":other, "password":password, "kb":kb["id"], "faq":faq["id"], "manual":manual["id"]}))
        check("r4.browser.real_native_ui_integration")
        return admin
    finally:
        secrets_file.unlink(missing_ok=True); server.shutdown(); server.server_close(); thread.join(timeout=5)
