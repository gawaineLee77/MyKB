"""Real pinned-native content/share/MCP checks; all content is synthetic."""
import time


def verify(request, check, call, rpc, role, owned, contributed, agent, key, admin, employee, tenant, other, docker):
    text = "# Synthetic R3 recovery\n\nThe synthetic recovery code is R3-VERIFY. Restart the synthetic worker and verify its health."
    manual = call(f"/api/v1/knowledge-bases/{owned['id']}/knowledge/manual", "POST",
                  {"title":"Synthetic R3 recovery", "content":text, "status":"publish"})["data"]
    deadline = time.monotonic() + 75
    while time.monotonic() < deadline:
        manual = call(f"/api/v1/knowledge/{manual['id']}")["data"]
        if manual["parse_status"] in ("completed", "failed"): break
        time.sleep(1)
    check("r3.document.manual_native_processing", manual["parse_status"] == "completed", status=manual["parse_status"])
    boundary = "r3-synthetic-file-boundary"
    payload = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="r3-synthetic.md"\r\nContent-Type: text/markdown\r\n\r\n{text}\r\n--{boundary}--\r\n').encode()
    uploaded = request(f"/api/v1/knowledge-bases/{owned['id']}/knowledge/file", "POST", token=admin, tenant=tenant,
                       headers={"Content-Type":"multipart/form-data; boundary="+boundary}, raw_body=payload)[0]["data"]
    deadline = time.monotonic() + 75
    while time.monotonic() < deadline:
        uploaded = call(f"/api/v1/knowledge/{uploaded['id']}")["data"]
        if uploaded["parse_status"] in ("completed", "failed"): break
        time.sleep(1)
    check("r3.document.native_multipart_upload", uploaded["file_name"] == "r3-synthetic.md" and uploaded["parse_status"] == "completed")
    request(f"/api/v1/knowledge/{manual['id']}/preview", token=employee, tenant=tenant)
    request(f"/api/v1/knowledge/{manual['id']}/download", token=employee, tenant=tenant, expected=(403,))
    role("contributor")
    request(f"/api/v1/knowledge/{manual['id']}/download", token=employee, tenant=tenant)
    role("viewer")
    check("r3.document.viewer_preview_contributor_download")
    faq = call("/api/v1/knowledge-bases", "POST", {"name":"R3 FAQ", "type":"faq"}, expected=(200,201))["data"]
    entry = {"standard_question":"What is the synthetic recovery code?", "answers":["R3-VERIFY"], "is_enabled":True}
    created = call(f"/api/v1/knowledge-bases/{faq['id']}/faq/entry", "POST", entry)["data"]
    call(f"/api/v1/knowledge-bases/{faq['id']}/faq/entries", token=employee)
    call(f"/api/v1/knowledge-bases/{faq['id']}/faq/entry", "POST", entry, token=employee, expected=(403,))
    check("r3.faq.native_create_read_write_authorization", bool(created))
    found = rpc("tools/call", {"name":"search_knowledge", "arguments":{"query":"synthetic recovery", "knowledge_base_ids":[owned["id"]]}}, key_record=key)
    rows = found.get("result",{}).get("structuredContent",{}).get("results",[])
    check("r3.mcp.native_search_processed_content", bool(rows))
    excerpt = rpc("tools/call", {"name":"get_source_excerpt", "arguments":{"chunk_id":rows[0]["id"]}}, key_record=key)
    check("r3.mcp.native_source_excerpt", bool(excerpt.get("result",{}).get("structuredContent",{}).get("content")))
    answer = rpc("tools/call", {"name":"ask_knowledge_agent", "arguments":{"query":"What is the synthetic recovery code?", "knowledge_base_ids":[owned["id"]]}}, key_record=key)
    value = answer.get("result",{}).get("structuredContent",{})
    check("r3.mcp.native_model_answer_and_bound_session", bool(value.get("answer")) and bool(value.get("session_id")) and bool(value.get("references")))
    call(f"/api/v1/sessions/{value['session_id']}", token=None, headers={"X-API-Key":key["token"]})
    unsafe = call("/api/v1/agents", "POST", {"name":"R3 unsafe tool agent", "config":{"agent_mode":"smart-reasoning", "kb_selection_mode":"selected", "knowledge_bases":[owned["id"]], "allowed_tools":["wiki_write_page"]}}, expected=(200,201))["data"]
    before = request("http://models:19090/counts")[0]
    blocked = rpc("tools/call", {"name":"ask_knowledge_agent", "arguments":{"query":"synthetic", "agent_id":unsafe["id"]}}, token=admin)
    check("r3.mcp.content_writing_agent_denied_before_model", "error" in blocked and before == request("http://models:19090/counts")[0])
    org = call("/api/v1/organizations", "POST", {"name":"R3 Synthetic Organization"}, expected=(200,201))["data"]
    call("/api/v1/organizations/join", "POST", {"invite_code":org["invite_code"]}, space=other, expected=(200,201))
    share = call(f"/api/v1/knowledge-bases/{owned['id']}/shares", "POST", {"organization_id":org["id"],"permission":"viewer"}, expected=(200,201))["data"]
    call(f"/api/v1/knowledge-bases/{owned['id']}", space=other)
    call(f"/api/v1/knowledge-bases/{owned['id']}", "PUT", {"name":"shared viewer denied"}, space=other, expected=(403,))
    bound = call("/api/v1/sessions", "POST", {"title":"R3 shared binding"}, space=other, expected=(200,201))["data"]["id"]
    # The native quick-answer stream creates a binding to this shared KB.
    call(f"/api/v1/knowledge-chat/{bound}", "POST", {"query":"synthetic recovery", "knowledge_base_ids":[owned["id"]], "disable_title":True}, space=other)
    call(f"/api/v1/knowledge-bases/{owned['id']}/shares/{share['id']}", "DELETE")
    before = request("http://models:19090/counts")[0]
    call(f"/api/v1/sessions/{bound}", space=other, expected=(403,404))
    call(f"/api/v1/sessions/continue-stream/{bound}", space=other, expected=(403,404))
    call(f"/api/v1/knowledge-chat/{bound}", "POST", {"query":"synthetic", "knowledge_base_ids":[owned["id"]]}, space=other, expected=(403,404))
    check("r3.share.revocation_denies_history_reconnect_and_model", before == request("http://models:19090/counts")[0])
    shared_agent = call(f"/api/v1/agents/{agent['id']}/shares", "POST", {"organization_id":org["id"],"permission":"viewer"}, expected=(200,201))["data"]
    call(f"/api/v1/agents/{agent['id']}", space=other, expected=(404,))
    call("/api/v1/shared-agents", space=other)
    call("/api/v1/mindcreek/agent/scope/resolve", "POST", {"agent_id":agent["id"]}, space=other)
    call(f"/api/v1/agents/{agent['id']}/shares/{shared_agent['id']}", "DELETE")
    call("/api/v1/mindcreek/agent/scope/resolve", "POST", {"agent_id":agent["id"]}, space=other, expected=(403,404))
    check("r3.agent.native_share_and_revocation")
    before = request("http://models:19090/counts")[0]
    call("/api/v1/messages/search", "POST", {"query":"synthetic", "mode":"vector"}, expected=(409,))
    check("r3.history.unscoped_vector_fails_before_model", before == request("http://models:19090/counts")[0])
    call(f"/api/v1/tenants/{tenant}", "PUT", {"memory_config":{"enabled":True}}, expected=(403,))
    check("r3.memory.disabled_native_feature_cannot_be_enabled_indirectly")

    user_id = call("/api/v1/auth/me", token=employee)["data"]["user"]["id"]
    import uuid
    uuid.UUID(user_id)
    subject = docker("exec","-T","postgres","psql","-U","r1","-d","r1","-Atc", "SELECT broker_subject FROM mindcreek.corporate_identities WHERE local_user_id='"+user_id+"'")
    call(f"/api/v1/mindcreek/identities/{subject}/suspend", "POST", {}, token=None, headers={"X-API-Key":key["token"]}, expected=(403,))
    call("/api/v1/mindcreek/models/builtin-mindcreek-chat/test", "POST", {}, token=None, headers={"X-API-Key":key["token"]}, expected=(403,))
    check("r3.machine.cannot_inherit_product_management_identity")
    call(f"/api/v1/mindcreek/identities/{subject}/suspend", "POST", {})
    before = request("http://models:19090/counts")[0]
    call("/api/v1/knowledge-search", "POST", {"query":"synthetic", "knowledge_base_ids":[owned["id"]]}, token=employee, expected=(403,))
    check("r3.identity.suspension_blocks_retrieval", before == request("http://models:19090/counts")[0])
    call(f"/api/v1/mindcreek/identities/{subject}/activate", "POST", {})
    call(f"/api/v1/tenants/{tenant}/members/{user_id}", "DELETE")
    call("/api/v1/knowledge-search", "POST", {"query":"synthetic", "knowledge_base_ids":[owned["id"]]}, token=employee, expected=(401,403))
    check("r3.mcp.employee_removal_does_not_revoke_independent_key", "result" in rpc("tools/list",key_record=key))
    # Explicit Owner restoration for the remaining R2 role/ownership regression.
    call(f"/api/v1/tenants/{tenant}/members", "POST", {"email":"r2-employee@example.invalid", "role":"viewer"}, expected=(201,))
    count = docker("exec","-T","postgres","psql","-U","r1","-d","r1","-Atc", "SELECT count(*) FROM mindcreek.kb_profiles")
    check("r3.data.native_operations_do_not_create_legacy_profiles", count == "0")

    persisted = docker("exec","-T","postgres","psql","-U","r1","-d","r1","-Atc", "SELECT count(*) FROM mindcreek.native_session_bindings")
    docker("restart","gateway")
    for attempt in range(30):
        try:
            call(f"/api/v1/sessions/{value['session_id']}", token=None, headers={"X-API-Key":key["token"]})
            break
        except (OSError, AssertionError):
            if attempt == 29: raise
            time.sleep(1)
    check("r3.migration.bindings_survive_gateway_restart", int(persisted)>0)
