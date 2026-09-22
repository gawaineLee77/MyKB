"""R3 extensions to the disposable real-native R2 regression harness."""
import json
import time


def configure(root, run, services, environment, gateway_env, report):
    from r3_evidence import inputs_digest
    report["r3_inputs_sha256"] = inputs_digest()
    report["scope"] = "R3 native workspace composition with fresh R2 regression; synthetic employees/models and isolated PostgreSQL; no real enterprise deployment acceptance"
    gateway_env.update({"MINDCREEK_NATIVE_WORKSPACE_ENABLED":"true", "MINDCREEK_NATIVE_ROUTES_FILE":"/config/r3-routes.json", "MINDCREEK_CAPABILITIES_FILE":"/config/r3-capabilities.json"})
    gateway_env["MINDCREEK_UPSTREAM_URL"] = "http://models:19091"
    models = []
    for suffix, kind in (("chat","KnowledgeQA"),("embedding","Embedding"),("rerank","Rerank")):
        parameters = {"base_url":"http://models:19090/v1", "api_key":"synthetic", "provider":"generic"}
        if suffix == "embedding": parameters["embedding_parameters"] = {"dimension":64}
        models.append({"id":"builtin-mindcreek-"+suffix, "name":"mindcreek-test-"+suffix, "type":kind, "source":"remote", "is_default":True, "status":"active", "parameters":parameters})
    model_file = run / "synthetic-models.yaml"
    model_file.write_text(json.dumps({"builtin_models":models}))
    environment.update({"BUILTIN_MODELS_CONFIG":"/fixture/models.yaml", "SSRF_WHITELIST_EXTRA":"identity,gateway,models"})
    services["app"]["volumes"] = [str(model_file)+":/fixture/models.yaml:ro",str(root/"deploy/phase5/builtin_agents.yaml")+":/app/config/builtin_agents.yaml:ro"]
    services["models"] = {"image":report["images"]["identity"]["id"],"command":["python","/fixture/r3_mock_models.py"],"volumes":[str(root/"testdata/redesign/r3_mock_models.py")+":/fixture/r3_mock_models.py:ro",str(root/"tools/phase0/mock_openai.py")+":/fixture/mock_openai.py:ro"]}


def resource_checks(request, check, oauth, cli, docker, tenant, other, admin, employee, employee_id, run):
    def call(path, method="GET", body=None, token=admin, space=tenant, expected=(200,), headers=None):
        return request(path, method, body, token=token, tenant=space, expected=expected, headers=headers)[0]
    def role(value): call(f"/api/v1/tenants/{tenant}/members/{employee_id}","PUT",{"role":value})
    def create(name, token=admin, space=tenant):
        return call("/api/v1/knowledge-bases","POST",{"name":name,"type":"document"},token,space,expected=(200,201))["data"]
    native_models = call("/api/v1/models")["data"]
    check("r3.models.synthetic_defaults_visible", len([m for m in native_models if m["id"].startswith("builtin-mindcreek-")])==3)
    owned = create("R3 Owner KB")
    check("r3.kb.native_create_without_profile",owned["embedding_model_id"]=="builtin-mindcreek-embedding" and owned["summary_model_id"]=="builtin-mindcreek-chat")
    foreign=create("R3 Other KB",space=other)
    role("contributor")
    contributed=create("R3 Contributor KB",token=employee)
    call(f"/api/v1/knowledge-bases/{owned['id']}","PUT",{"name":"forbidden"},token=employee,expected=(403,))
    call(f"/api/v1/knowledge-bases/{contributed['id']}","PUT",{"name":"contributor edited"},token=employee)
    check("r3.contributor.owns_only_own_writes")
    role("viewer")
    call("/api/v1/knowledge-bases","POST",{"name":"viewer denied"},token=employee,expected=(403,))
    call(f"/api/v1/knowledge-bases/{contributed['id']}","PUT",{"name":"downgraded creator"},token=employee)
    call(f"/api/v1/knowledge-bases/{owned['id']}","PUT",{"name":"forbidden"},token=employee,expected=(403,))
    check("r3.viewer.native_downgraded_creator_exception")
    role("admin")
    call(f"/api/v1/knowledge-bases/{owned['id']}","PUT",{"name":"space admin edited"},token=employee)
    check("r3.admin.native_resource_management")
    role("viewer")
    outsider=oauth("r3-outsider")
    request("/api/v1/mindcreek/onboarding","POST",{},token=outsider["token"])
    call(f"/api/v1/knowledge-bases/{foreign['id']}",token=outsider["token"],expected=(403,404))
    before=request("http://models:19090/counts")[0]
    call("/api/v1/knowledge-search","POST",{"query":"synthetic", "knowledge_base_ids":[owned["id"],foreign["id"]]},token=outsider["token"],expected=(403,404))
    after=request("http://models:19090/counts")[0]
    check("r3.scope.mixed_foreign_kb_rejected_before_model",before==after)
    session = call("/api/v1/sessions","POST",{"title":"R3 employee session"},token=outsider["token"],expected=(200,201))["data"]["id"]
    call(f"/api/v1/sessions/{session}",token=employee,expected=(403,))
    check("r3.session.other_employee_denied")
    scoped = call("/api/v1/sessions",token=outsider["token"])
    check("r3.session.bound_list",scoped["total"]==1 and scoped["data"][0]["id"]==session)
    agent = call("/api/v1/agents","POST",{"name":"R3 Selected Agent","config":{"agent_mode":"quick-answer","kb_selection_mode":"selected","knowledge_bases":[owned["id"]]}},expected=(200,201))["data"]
    check("r3.agent.native_models_defaulted",agent["config"]["model_id"]=="builtin-mindcreek-chat")
    before=request("http://models:19090/counts")[0]
    call(f"/api/v1/knowledge-chat/{session}","POST",{"query":"synthetic","agent_id":agent["id"],"knowledge_base_ids":[contributed["id"]]},token=outsider["token"],expected=(403,))
    after=request("http://models:19090/counts")[0]
    check("r3.agent.explicit_cannot_expand_server_scope",before==after)
    call("/api/v1/agents","POST",{"name":"forbidden binding","config":{"knowledge_bases":[foreign["id"]]}},expected=(403,404))
    check("r3.agent.cross_space_binding_rejected")
    call("/api/v1/knowledge-bases","POST",{"name":"invalid model","summary_model_id":"unavailable-model"},expected=(400,404))
    call("/api/v1/knowledge-bases","POST",{"name":"graph unavailable","indexing_strategy":{"graph_enabled":True}},expected=(409,))
    check("r3.models.invalid_explicit_and_missing_graph_dependencies_rejected")
    wiki=call("/api/v1/knowledge-bases","POST",{"name":"R3 Native Wiki","type":"document","indexing_strategy":{"vector_enabled":False,"keyword_enabled":False,"wiki_enabled":True},"wiki_config":{"enabled":True}},expected=(200,201))["data"]
    check("r3.wiki.native_indexing_preserved",wiki["indexing_strategy"]["wiki_enabled"] and not wiki["indexing_strategy"]["vector_enabled"])
    def key(name,caps,kbs):
        return call(f"/api/v1/tenants/{tenant}/api-keys","POST",{"name":name,"full_access":False,"capabilities":caps,"knowledge_base_ids":kbs},expected=(201,))["data"]
    key_a=key("R3 retrieve chat A",["retrieve","chat","read_agents"],[owned["id"]])
    key_b=key("R3 retrieve chat B",["retrieve","chat","read_agents"],[owned["id"]])
    key_limited=key("R3 agent read only",["read_agents"],[owned["id"]])
    def rpc(method,args=None,key_record=None,token=None):
        headers={"MCP-Protocol-Version":"2025-11-25"}
        if key_record:headers["X-API-Key"]=key_record["token"]
        return call("/mcp","POST",{"jsonrpc":"2.0","id":1,"method":method,"params":args or {}},token=token,headers=headers)
    request("/mcp","POST",{"jsonrpc":"2.0","id":1,"method":"tools/list"},expected=(401,))
    discovered=rpc("tools/list",key_record=key_a)
    names={tool["name"] for tool in discovered["result"]["tools"]}
    check("r3.mcp.authenticated_four_tools",names=={"list_knowledge_bases","search_knowledge","get_source_excerpt","ask_knowledge_agent"})
    removed=rpc("tools/call",{"name":"list_publications","arguments":{}},key_record=key_a)
    check("r3.mcp.removed_tool_standard_error",removed["error"]["code"]==-32602)
    listed=rpc("tools/call",{"name":"list_knowledge_bases","arguments":{}},key_record=key_a)
    check("r3.mcp.machine_native_kb_scope",listed["result"]["structuredContent"]["knowledge_base_ids"]==[owned["id"]])
    denied=rpc("tools/call",{"name":"search_knowledge","arguments":{"query":"synthetic","knowledge_base_ids":[contributed["id"]]}},key_record=key_a)
    insufficient=rpc("tools/call",{"name":"search_knowledge","arguments":{"query":"synthetic"}},key_record=key_limited)
    check("r3.mcp.machine_scope_and_capability_denied","error" in denied and "error" in insufficient)
    machine_session=call("/api/v1/sessions","POST",{"title":"R3 machine A"},token=None,headers={"X-API-Key":key_a["token"]},expected=(200,201))["data"]["id"]
    call(f"/api/v1/sessions/{machine_session}",token=None,headers={"X-API-Key":key_b["token"]},expected=(403,))
    call(f"/api/v1/sessions/{machine_session}",expected=(403,))
    check("r3.session.machine_key_and_human_isolation")
    call(f"/api/v1/tenants/{tenant}/api-keys/{key_a['id']}","DELETE")
    request("/mcp","POST",{"jsonrpc":"2.0","id":1,"method":"tools/list"},headers={"X-API-Key":key_a["token"],"MCP-Protocol-Version":"2025-11-25"},tenant=tenant,expected=(401,))
    still_valid=rpc("tools/list",key_record=key_b)
    check("r3.mcp.key_revocation_is_independent","result" in still_valid)
    from r3_resources import verify
    verify(request, check, call, rpc, role, owned, contributed, agent, key_b, admin, employee, tenant, other, docker)
    # Native resource writes are authorized by their own verb-specific handlers.
    call(f"/api/v1/agents/{agent['id']}","DELETE",token=outsider["token"],expected=(403,))
    call(f"/api/v1/agents/{agent['id']}","DELETE")
    call(f"/api/v1/knowledge-bases/{wiki['id']}","DELETE")
    check("r3.resources.native_delete_permissions")
    for path in ("/api/v1/knowledge-spaces", f"/api/v1/knowledge-bases/{owned['id']}/notes", "/api/v1/mindcreek/catalog", "/api/v1/mindcreek/me/subscriptions", f"/api/v1/knowledge-bases/{owned['id']}/product-profile"):
        result=call(path,expected=(410,))
        check("r3.retired."+path.rsplit("/",1)[-1],result.get("error",{}).get("code")=="feature.retired")
    call("/api/v1/knowledge-bases/unknown/unclassified",expected=(404,))
    check("r3.routes.unknown_denied")
    role("contributor")
    cli("migrate","down","1",expected=1)
    check("r3.migration.populated_audit_refuses_rollback")
    # Native removal invalidates previous employee JWTs even after explicit re-add.
    return oauth("r2-employee")["token"]


if __name__ == "__main__":
    import sys
    from r2_probe import main
    sys.argv.extend(["--profile","r3"])
    main()
