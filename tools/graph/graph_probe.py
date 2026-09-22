"""Graph extension to the fresh R2 regression: native app, Neo4j/APOC and synthetic LLM."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import threading
import time


def configure_graph(root, run, services, report):
    spec = importlib.util.spec_from_file_location('graph_deployment', root / 'deploy/graph/graph.py')
    deployment = importlib.util.module_from_spec(spec); spec.loader.exec_module(deployment)
    service = deployment.graph_service('f' * 48)
    # Compose's random project prefixes this fresh volume; the harness removes only its own volume.
    services['neo4j'] = service
    for value in services.values(): value['platform'] = 'linux/amd64'
    services['app']['environment'].update(NEO4J_ENABLE='true', NEO4J_URI='bolt://neo4j:7687',
        NEO4J_USERNAME='neo4j', NEO4J_PASSWORD='f' * 48, LOG_LEVEL='warn', LLM_DEBUG_LOG='false')
    services['app']['depends_on']['neo4j'] = {'condition': 'service_healthy'}
    env = services['gateway']['environment']
    env.update(MINDCREEK_GRAPH_ENABLED='true', MINDCREEK_CAPABILITIES_FILE='/graph/capabilities.json',
        MINDCREEK_UPSTREAM_URL='http://app:8080', MINDCREEK_UPSTREAM_TIMEOUT='120s')
    caps = json.loads((root / 'config/r3-capabilities.json').read_text())
    caps['capabilities']['rag_graph'] = True
    (run / 'graph-capabilities.json').write_text(json.dumps(caps))
    services['gateway']['volumes'].append(str(run / 'graph-capabilities.json') + ':/graph/capabilities.json:ro')
    services['models']['command'] = ['python', '/fixture/models.py']
    services['models']['volumes'] = [str(root / 'testdata/graph/models.py') + ':/fixture/models.py:ro',
        str(root / 'tools/phase0/mock_openai.py') + ':/fixture/mock_openai.py:ro']
    report['images']['neo4j'] = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', deployment.IMAGE]))[0]['Id']
    report['scope'] = 'AMD64 emulation: native WeKnora v0.8.0, real Neo4j/APOC and product gateway, isolated synthetic OAuth and deterministic LLM. Not enterprise model extraction quality.'


def graph_checks(root, run, request, check, oauth, docker, tenant, other, admin, employee, employee_id, base):
    def call(path, method='GET', body=None, token=admin, space=tenant, expected=(200,), headers=None):
        return request(path, method, body, token=token, tenant=space, expected=expected, headers=headers)[0]
    def cypher(query):
        return docker('exec', '-T', 'neo4j', 'sh', '-c',
            'export NEO4J_USERNAME=neo4j NEO4J_PASSWORD="${NEO4J_AUTH#neo4j/}"; exec cypher-shell -a bolt://localhost:7687 --format plain "$1"', '_', query)
    def scalar(query):
        return int(cypher(query).splitlines()[-1].strip('" '))
    check('graph.neo4j_system_info', call('/api/v1/system/info')['data']['graph_database_engine'] == 'Neo4j')
    check('graph.apoc_offline_loaded', '2025.10' in cypher('RETURN apoc.version()'))
    check('graph.native_example_tags', bool(call('/api/v1/initialization/extract/fabri-tag', 'POST', {})['data']['tags']))
    check('graph.native_example_text', bool(call('/api/v1/initialization/extract/fabri-text', 'POST', {'tags': ['DEPENDS_ON']})['data']['text']))
    preview = call('/api/v1/initialization/extract/text-relation', 'POST',
        {'text': 'Astra depends on Helios.', 'tags': ['DEPENDS_ON'], 'model_id': 'builtin-mindcreek-chat'})['data']
    check('graph.native_extraction_preview', len(preview['nodes']) == 2 and len(preview['relations']) == 1)
    def create(name, text, space=tenant):
        config = {'enabled': True, 'tags': ['DEPENDS_ON'], 'text': 'Astra depends on Helios.',
            'nodes': preview['nodes'], 'relations': preview['relations']}
        kb = call('/api/v1/knowledge-bases', 'POST', {'name': name, 'type': 'document',
            'indexing_strategy': {'vector_enabled': False, 'keyword_enabled': False, 'wiki_enabled': False, 'graph_enabled': True},
            'extract_config': config}, space=space, expected=(200, 201))['data']
        doc = call(f"/api/v1/knowledge-bases/{kb['id']}/knowledge/manual", 'POST',
            {'title': name + ' document', 'content': '# Synthetic graph\n\n' + text, 'status': 'publish'}, space=space)['data']
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            current = call('/api/v1/knowledge/' + doc['id'], space=space)['data']
            if current['parse_status'] in ['completed', 'failed']: break
            time.sleep(2)
        check('graph.document_extraction_completed.' + name, current['parse_status'] == 'completed')
        return kb, doc
    kb, doc = create('Graph-A', 'Astra depends on Helios. This is the authorized synthetic project.')
    foreign, foreign_doc = create('Graph-B', 'Boreal depends on Vault. This is another synthetic space.', other)
    count_query = "MATCH (n {kg: '" + doc['id'] + "'}) RETURN count(n)"
    check('graph.native_persisted_nodes', scalar(count_query) == 2)
    check('graph.native_persisted_edge', scalar("MATCH (n {kg: '" + doc['id'] + "'})-[r:DEPENDS_ON]->() RETURN count(r)") == 1)
    # A graph-only KB proves source retrieval did not fall back to vector/BM25.
    check('graph.only_index_config_preserved', not kb['indexing_strategy']['vector_enabled'] and not kb['indexing_strategy']['keyword_enabled'])
    actor = oauth('graph-viewer')
    call('/api/v1/mindcreek/onboarding', 'POST', {}, token=actor['token'])
    def rpc(arguments, token=actor['token'], headers=None):
        return call('/mcp', 'POST', {'jsonrpc': '2.0', 'id': 1, 'method': 'tools/call',
            'params': {'name': 'ask_knowledge_agent', 'arguments': arguments}}, token=token,
            headers={'MCP-Protocol-Version': '2025-11-25', **(headers or {})})
    answer = rpc({'query': 'What does Astra depend on?', 'knowledge_base_ids': [kb['id']]})
    data = answer.get('result', {}).get('structuredContent', {})
    if not data.get('references'): (run / 'graph-answer.json').write_text(json.dumps(answer))
    check('graph.mcp_answer_has_graph_only_source', bool(data.get('answer')) and bool(data.get('references')))
    check('graph.source_is_authorized_document', all(r['knowledge_id'] == doc['id'] for r in data['references']))
    agent = call('/api/v1/agents', 'POST', {'name': 'Graph assistant', 'config': {
        'agent_mode': 'quick-answer', 'kb_selection_mode': 'selected', 'knowledge_bases': [kb['id']]}}, expected=(200, 201))['data']
    result = rpc({'query': 'What does Astra depend on?', 'agent_id': agent['id']})
    agent_answer = result.get('result', {}).get('structuredContent', {})
    if not agent_answer.get('references'): (run / 'graph-agent-answer.json').write_text(json.dumps(result))
    check('graph.native_agent_answer_with_sources', bool(agent_answer.get('answer')) and bool(agent_answer.get('references')))
    before = request('http://models:19090/counts')[0]
    call('/api/v1/initialization/extract/text-relation', 'POST',
        {'text': 'Astra depends on Helios.', 'tags': ['DEPENDS_ON']}, token=actor['token'], expected=(403,))
    rejected = rpc({'query': 'Tell me about both spaces', 'knowledge_base_ids': [kb['id'], foreign['id']]})
    call('/api/v1/knowledge-bases', 'POST', {'name': 'viewer cannot create'}, token=actor['token'], expected=(403,))
    call('/api/v1/knowledge-bases/' + kb['id'], 'PUT', {'extract_config': {'enabled': False}}, token=actor['token'], expected=(403,))
    after = request('http://models:19090/counts')[0]
    check('graph.cross_space_denied_before_model', 'error' in rejected and before == after)
    key = call(f'/api/v1/tenants/{tenant}/api-keys', 'POST', {'name': 'graph restricted key', 'full_access': False,
        'capabilities': ['retrieve', 'chat', 'read_agents'], 'knowledge_base_ids': [kb['id']]}, expected=(201,))['data']
    machine = rpc({'query': 'What does Astra depend on?', 'knowledge_base_ids': [kb['id']]}, token=None, headers={'X-API-Key': key['token']})
    check('graph.machine_key_scoped_answer', bool(machine.get('result', {}).get('structuredContent', {}).get('references')))
    before = request('http://models:19090/counts')[0]
    denied = rpc({'query': 'Boreal', 'knowledge_base_ids': [foreign['id']]}, token=None, headers={'X-API-Key': key['token']})
    check('graph.machine_key_foreign_denied', 'error' in denied and before == request('http://models:19090/counts')[0])
    docker('restart', 'neo4j')
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            if scalar(count_query) == 2: break
        except RuntimeError: pass
        time.sleep(3)
    check('graph.restart_preserves_data', scalar(count_query) == 2)
    # Reparse with native API must not duplicate entities/relationships.
    call('/api/v1/knowledge/' + doc['id'] + '/reparse', 'POST', {})
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        status = call('/api/v1/knowledge/' + doc['id'])['data']['parse_status']
        if status in ['completed', 'failed']: break
        time.sleep(2)
    check('graph.reparse_idempotent', status == 'completed' and scalar(count_query) == 2)
    # Keep a running test server only while the bounded browser checks execute.
    if os.environ.get('GRAPH_BROWSER') == '1':
        from graph_browser import verify_browser
        verify_browser(root, run, base, admin, actor['token'], tenant, kb['id'], agent['id'])
    call('/api/v1/knowledge/' + doc['id'], 'DELETE')
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        if scalar(count_query) == 0: break
        time.sleep(2)
    check('graph.delete_cleans_only_target_document', scalar(count_query) == 0 and scalar("MATCH (n {kg: '" + foreign_doc['id'] + "'}) RETURN count(n)") == 2)
