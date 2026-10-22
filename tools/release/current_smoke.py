"""Current AMD64 fresh/direct-upgrade checks; disposable synthetic data only."""
import base64
import datetime
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import secrets
import struct
import time
import urllib.parse
import zlib
from release_baselines import SOURCE_RELEASES, source_has_knowledge_acl, upgrade_key_check

ROOT = Path(__file__).resolve().parents[2]
CURRENT_WORKDIR = Path(os.environ.get('MINDCREEK_CURRENT_WORKDIR', ROOT / '.local/enterprise-current'))


def load(path, name):
    loader = importlib.machinery.SourceFileLoader(name, str(path))
    spec = importlib.util.spec_from_loader(name, loader)
    module = importlib.util.module_from_spec(spec)
    loader.exec_module(module)
    return module


def graph_query(dep, sql):
    return dep.compose(['exec', '-T', 'neo4j', 'sh', '-c',
        'NEO4J_USERNAME=neo4j NEO4J_PASSWORD="${NEO4J_AUTH#neo4j/}" cypher-shell -a bolt://localhost:7687 "$1"', 'sh', sql], True)


def database_facts(dep):
    sql = ('SELECT count(*) FROM users; SELECT count(*) FROM knowledge_bases; '
           'SELECT count(*) FROM knowledges; SELECT max(version) FROM mindcreek.schema_migrations; '
           "SELECT to_regclass('mindcreek.assistant_sessions') IS NOT NULL")
    values = dep.compose(['exec', '-T', 'postgres', 'psql', '-U', 'mindcreek', '-d', 'mindcreek_r4', '-Atc', sql], True).strip().splitlines()
    return dict(users=int(values[0]), knowledge_bases=int(values[1]), documents=int(values[2]),
                migration_version=int(values[3]), r5_schema_present=values[4] == 't')


def prepare_old_addons(g):
    """Give each supported source its own released graph configuration."""
    state, root, dep = g['STATE'], g['ROOT'], g['dep']
    release = dep.release()['release']
    if release not in SOURCE_RELEASES:
        raise ValueError('Direct-upgrade smoke requires an explicitly supported source release')
    if release != 'r4-20260916-118b8af3':
        ops = load(g['PACKAGE'] / 'bin/release', 'source_release_ops')
        ops.graph(dep, True)
        links = state / 'ui-links'
        links.mkdir(exist_ok=True)
        (links / 'links.json').write_text('{"version":1,"enabled":false,"links":{}}\n')
        return
    graph = load(root / 'deploy/graph/graph.py', 'old_graph_fixture')
    directory = state / 'graph'
    directory.mkdir(mode=0o711)
    password = secrets.token_hex(24)
    (directory / 'password').write_text(password + '\n')
    (directory / 'password').chmod(0o600)
    current = json.loads((state / 'compose.override.json').read_text())
    updated = graph.configure(current, state, password, True)
    updated['services']['app']['depends_on'] = {'neo4j': {'condition': 'service_healthy'}}
    links = state / 'ui-links'
    links.mkdir()
    (links / 'links.json').write_text('{"version":1,"enabled":false,"links":{}}\n')
    updated['services']['frontend'] = {'image': 'mindcreek-ui:r4-20260918-links1',
        'volumes': [str(links) + ':/usr/share/nginx/html/mindcreek-runtime:ro']}
    (state / 'compose.override.json').write_text(json.dumps(updated))
    (state / 'compose.override.json').chmod(0o600)
    caps = json.loads((g['PACKAGE'] / 'config/r3-capabilities.json').read_text())
    caps['capabilities']['rag_graph'] = True
    (directory / 'capabilities.json').write_text(json.dumps(caps))
    (directory / 'routes.json').write_bytes((root / 'images/archives/mindcreek-r4-20260916-graph1-amd64-addon/routes.json').read_bytes())


def publish_synthetic_kb(g):
    """Prove the secure default, then explicitly publish this synthetic fixture."""
    request, check = g['request'], g['check']
    tenant, kb, token, employee = g['tenant'], g['kb'], g['token'], g['employee_token']
    route = '/api/v1/mindcreek/kb-permissions/' + kb['id']
    policy = request(route, token=token, tenant=tenant)[0]['data']
    check('current_kb_default_creator_private', policy['visibility'] == 'restricted' and bool(policy['creator_user_id']))
    request('/api/v1/knowledge/' + g['upload']['id'], token=employee, tenant=tenant, expected=(404,))
    request(route, 'PUT', {'revision': policy['revision'], 'visibility': 'workspace', 'members': []}, token=token, tenant=tenant)
    request('/api/v1/chunks/' + g['upload']['id'], token=employee, tenant=tenant)
    request('/api/v1/knowledge/' + g['upload']['id'] + '/preview', token=employee, tenant=tenant, expected=(403,))
    check('current_public_kb_viewer_content_without_original_file')


def png_fixture():
    def chunk(kind, value):
        return struct.pack('>I', len(value)) + kind + value + struct.pack('>I', zlib.crc32(kind + value) & 0xffffffff)
    return b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>2I5B', 1, 1, 8, 6, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(b'\0\x15\x70\xd0\xff')) + chunk(b'IEND', b'')


def key_secret(key):
    value = key.get('token') or key.get('api_key')
    if not value:
        raise AssertionError('Synthetic key response omitted credential')
    return value


def current_features(g, call, agent_config):
    request, check = g['request'], g['check']
    token, employee, tenant, kb = g['token'], g['employee_token'], g['tenant'], g['kb']
    png = png_fixture()
    brand = request('/api/v1/mindcreek/branding')[0]['data']
    check('current_global_branding_default', brand['name'] == '小纳')
    changed = call('/api/v1/mindcreek/branding', 'PUT', {'revision': brand['revision'], 'name': 'Synthetic 小纳 $&',
        'logo_data': 'data:image/png;base64,' + base64.b64encode(png).decode()})['data']
    call('/api/v1/mindcreek/branding', 'PUT', {'revision': changed['revision'], 'name': 'Unauthorized'}, who=employee, expected=(403,))
    public = request('/api/v1/mindcreek/branding')[0]['data']
    _, headers = request(public['logo_url'])
    check('current_global_branding_local_admin_and_public_logo', public['name'] == 'Synthetic 小纳 $&' and headers.get('Content-Type') == 'image/png')
    avatar = request('/api/v1/mindcreek/agent-icons', 'POST', token=token, tenant=tenant,
        headers={'Content-Type': 'image/png'}, raw=png, expected=(201,))[0]['data']['avatar']
    request('/api/v1/mindcreek/agent-icons', 'POST', token=employee, tenant=tenant,
        headers={'Content-Type': 'image/png'}, raw=png, expected=(403,))
    agent = call('/api/v1/agents', 'POST', {'name': 'Current package assistant', 'avatar': avatar, 'config': agent_config}, expected=(200, 201))['data']
    icon = '/api/v1/mindcreek/agent-icons/' + avatar.removeprefix('mc-avatar:')
    request(icon + '?agent_id=' + agent['id'], token=token, tenant=tenant)
    request(icon, token=employee, tenant=tenant, expected=(403, 404))
    check('current_agent_icon_upload_reference_and_draft_isolation', agent['avatar'] == avatar)
    g['current_feature_facts'] = {'brand_name': public['name'], 'brand_revision': public['revision'], 'avatar': avatar, 'agent_id': agent['id']}
    rpc = lambda name, args: request('/mcp', 'POST', {'jsonrpc': '2.0', 'id': 1, 'method': 'tools/call',
        'params': {'name': name, 'arguments': args}}, token=employee, tenant=tenant, headers={'MCP-Protocol-Version': '2025-11-25'})[0]
    listed = request('/mcp', 'POST', {'jsonrpc': '2.0', 'id': 1, 'method': 'tools/list', 'params': {}},
        token=employee, tenant=tenant, headers={'MCP-Protocol-Version': '2025-11-25'})[0]['result']['tools']
    names = {t['name'] for t in listed}
    check('current_six_mcp_tools', len(names) == 6 and {'wiki_search', 'wiki_read_page'} <= names)
    current_kb = call('/api/v1/knowledge-bases/' + kb['id'])['data']
    indexing = {**current_kb['indexing_strategy'], 'wiki_enabled': True}
    call('/api/v1/knowledge-bases/' + kb['id'], 'PUT', {'name': kb['name'], 'config': {'indexing_strategy': indexing}})
    call('/api/v1/knowledgebase/' + kb['id'] + '/wiki/pages', 'POST', {'slug': 'synthetic-upgrade',
        'title': 'Synthetic upgrade Wiki', 'content': '# Synthetic upgrade\n\nWIKI-VERIFIED synthetic only.',
        'summary': 'WIKI-VERIFIED', 'page_type': 'concept', 'status': 'published', 'source_refs': [], 'chunk_refs': []}, expected=(201,))
    found = rpc('wiki_search', {'query': 'WIKI-VERIFIED', 'knowledge_base_ids': [kb['id']]})
    read = rpc('wiki_read_page', {'knowledge_base_id': kb['id'], 'slug': 'synthetic-upgrade'})
    check('current_wiki_mcp_search_and_read', 'WIKI-VERIFIED' in json.dumps(found) and 'WIKI-VERIFIED' in read.get('result', {}).get('structuredContent', {}).get('content', ''))
    private = g['faq']['id']
    denied = rpc('wiki_search', {'query': 'WIKI-VERIFIED', 'knowledge_base_ids': [private]})
    empty = rpc('wiki_search', {'query': 'WIKI-VERIFIED', 'knowledge_base_ids': []})
    check('current_wiki_mcp_private_and_empty_scope_denied', bool(denied.get('error') or denied.get('result', {}).get('isError')) and bool(empty.get('error') or empty.get('result', {}).get('isError')))
    # Sessions must be distinct from the interactive Owner login. A boundary
    # second can briefly return the documented retry code; never accept others.
    for route in ('/api/v1/mindcreek/workspace-admin/delegation', '/api/v1/mindcreek/kb-collaboration/delegation'):
        for attempt in range(4):
            status = call(route, 'POST', {}, expected=(200, 409))
            if status.get('success'):
                break
            if status.get('error', {}).get('code') != 'workspace_admin.delegation_retry':
                raise AssertionError('Unexpected delegation failure: ' + route)
            time.sleep(1.1)
        check('current_' + ('workspace_admin' if 'workspace-admin' in route else 'kb_collaboration') + '_explicit_delegation', status.get('success') and status['data'].get('authorized'))
    key_route = f'/api/v1/tenants/{tenant}/api-keys'
    key_input = {'name': 'Synthetic bounded key', 'full_access': False,
                 'knowledge_base_ids': [kb['id']], 'capabilities': ['retrieve'], 'knowledge_operations': []}
    if g.get('legacy_key'):
        old_key = g['legacy_key']
        if not g.get('preupgrade_key_mapped'):
            request('/api/v1/knowledge-bases/' + kb['id'], tenant=tenant,
                    headers={'X-API-Key': key_secret(old_key)}, expected=(403, 404))
            call(key_route + '/' + str(old_key['id']), 'PUT', key_input)
        request('/api/v1/knowledge-bases/' + kb['id'], tenant=tenant, headers={'X-API-Key': key_secret(old_key)})
        request('/api/v1/knowledge-bases/' + private, tenant=tenant,
                headers={'X-API-Key': key_secret(old_key)}, expected=(403, 404))
        check(upgrade_key_check(g['report']['upgrade_from']))
    owner_key = call(key_route, 'POST', key_input, expected=(201,))['data']
    identity = call('/api/v1/auth/me', who=employee)['data']['user']['id']
    member_route = f'/api/v1/tenants/{tenant}/members/{identity}'
    call(member_route, 'PUT', {'role': 'admin'})
    admin_key = call(key_route, 'POST', {**key_input, 'name': 'Synthetic Admin own key'}, who=employee, expected=(201,))['data']
    own_keys = call(key_route, who=employee)['data']
    all_keys = call(key_route)['data']
    check('current_admin_own_keys_owner_all_keys',
          admin_key['id'] in [k['id'] for k in own_keys] and owner_key['id'] not in [k['id'] for k in own_keys]
          and {admin_key['id'], owner_key['id']} <= {k['id'] for k in all_keys})
    request('/api/v1/knowledge-bases/' + kb['id'], tenant=tenant, headers={'X-API-Key': key_secret(admin_key)})
    request('/api/v1/knowledge-bases/' + private, tenant=tenant,
            headers={'X-API-Key': key_secret(admin_key)}, expected=(403, 404))
    call('/api/v1/mindcreek/branding', 'PUT', {'revision': changed['revision'], 'name': 'Unauthorized Admin'}, who=employee, expected=(403,))
    call(member_route, 'PUT', {'role': 'viewer'})
    request('/api/v1/knowledge-bases/' + kb['id'], tenant=tenant,
            headers={'X-API-Key': key_secret(admin_key)}, expected=(401, 403, 404))
    check('current_machine_key_scope_and_role_revocation')
    return agent


def additional_checks(g):
    dep, request, check = g['dep'], g['request'], g['check']
    state, local = g['STATE'], g['LOCAL']
    token, employee, tenant, kb, document = g['token'], g['employee_token'], g['tenant'], g['kb'], g['upload']
    target = Path((CURRENT_WORKDIR / 'package-path').read_text().strip())
    ops = load(target / 'bin/release', 'current_release_ops')
    if os.environ.get('MINDCREEK_UPGRADE_SMOKE') == 'true':
        from image_resources_smoke import prepare_history
        prepare_history(g)
        old = dep
        dep = load(target / 'bin/mindcreek', 'upgrade_target')
        source = old.release()['release']
        existing_acl = source_has_knowledge_acl(source)
        g['preupgrade_key_mapped'] = existing_acl
        if existing_acl:
            previous_policy = request('/api/v1/mindcreek/kb-permissions/' + kb['id'], token=token, tenant=tenant)[0]['data']
        before_env = old.read_env()
        fingerprints = {name: ops.sha(state / name) for name in ('secrets/admin-password', 'graph/password', 'ui-links/links.json')}
        facts = database_facts(old)
        check('source_expected_product_migrations', facts['migration_version'] == SOURCE_RELEASES[source], source_release=source, migration_version=facts['migration_version'])
        if existing_acl:
            # A 1019 installation already enforces delegated API-key management.
            # The synthetic Owner explicitly authorizes it before creating the
            # mapped key whose preservation this upgrade is meant to test.
            for attempt in range(4):
                delegated = request('/api/v1/mindcreek/workspace-admin/delegation', 'POST', {},
                                    token=token, tenant=tenant, expected=(200, 409))[0]
                if delegated.get('success'):
                    break
                if delegated.get('error', {}).get('code') != 'workspace_admin.delegation_retry':
                    raise AssertionError('Unexpected source workspace delegation failure')
                time.sleep(1.1)
            check('source_existing_acl_key_delegation_authorized',
                  delegated.get('success') and delegated['data'].get('authorized'))
        g['legacy_key'] = request(f'/api/v1/tenants/{tenant}/api-keys', 'POST',
            {'name': 'Synthetic pre-upgrade key', 'full_access': False, 'knowledge_base_ids': [kb['id']], 'capabilities': ['retrieve']},
            token=token, tenant=tenant, expected=(200, 201))[0]['data']
        request('/api/v1/knowledge-bases/' + kb['id'], tenant=tenant,
                headers={'X-API-Key': key_secret(g['legacy_key'])})
        check('source_legacy_key_can_read_selected_kb')
        graph_query(old, "CREATE (:CURRENT_UPGRADE_MARKER {value:'preserved-synthetic-graph'})")
        g['report']['source_database_facts'] = facts
        proposed = ops.plan(old.ROOT, dep)[2]
        check('upgrade_recognizes_source_release_and_graph', proposed['from_release'] == source
              and proposed['to_release'] == dep.release()['release'] and 'mindcreek_graph_data' in proposed['volumes'])
        ops.upgrade(dep, old.ROOT, state / 'backup/before-upgrade')
        after_env = dep.read_env()
        check('upgrade_preserves_identity_models_password_graph_and_links',
            all(ops.sha(state / name) == digest for name, digest in fingerprints.items()) and
            all(after_env.get(k) == v for k, v in before_env.items() if k != 'MINDCREEK_WORKSPACE_ADMIN_SECRET_FILE'))
        g['dep'], g['PACKAGE'] = dep, target
        g['cmd'] = [str(x).replace(str(old.ROOT), str(target)) for x in g['cmd']]
        g['report'].update(release=dep.release()['release'], package_runtime_sha256=dep.release()['package_runtime_sha256'],
                           upgrade_from=source, source_images=g['report']['images'], images=dep.release()['images'])
        preserved = 'preserved-synthetic-graph' in graph_query(dep, 'MATCH (n:CURRENT_UPGRADE_MARKER) RETURN n.value')
        check('upgrade_preserves_preexisting_graph_marker', preserved)
        marker = {'status': 'passed', 'release': dep.release()['release'], 'upgrade_from': source,
                  'source_release': source, 'package_runtime_sha256': dep.release()['package_runtime_sha256'],
                  'project': dep.PROJECT, 'old_graph_seeded_before_backup': True, 'graph_preserved_after_upgrade': preserved,
                  'source_database_facts': facts, 'time': datetime.datetime.now(datetime.timezone.utc).isoformat()}
        (local / 'upgrade-marker.json').write_text(json.dumps(marker, indent=2) + '\n')
        request('/api/v1/auth/me', token=token, tenant=tenant)
        request('/api/v1/auth/me', token=employee, tenant=tenant)
        request('/api/v1/knowledge/' + document['id'] + '/preview', token=token, tenant=tenant)
        check('upgrade_old_admin_employee_sessions_and_owner_document_survive')
        if existing_acl:
            current_policy = request('/api/v1/mindcreek/kb-permissions/' + kb['id'], token=token, tenant=tenant)[0]['data']
            check('upgrade_existing_kb_policy_preserved', current_policy == previous_policy)
            request('/api/v1/chunks/' + document['id'], token=employee, tenant=tenant)
            request('/api/v1/knowledge/' + document['id'] + '/preview', token=employee, tenant=tenant, expected=(403,))
        else:
            publish_synthetic_kb(g)
    ops.graph(dep, True)
    values = dep.read_env()
    values['MINDCREEK_EMPLOYEE_ASSISTANT_ENABLED'] = 'true'
    (state / 'enterprise.env').write_text(''.join(k + '=' + v + '\n' for k, v in values.items()))
    dep.validate()
    dep.up_services(['neo4j', 'app', 'gateway'])
    check('current_optional_neo4j_apoc_healthy')
    def call(path, method='GET', body=None, who=token, headers=None, expected=(200,)):
        return request(path, method, body, token=who, tenant=tenant, headers=headers, expected=expected)[0]
    graph_config = {**kb['indexing_strategy'], 'graph_enabled': True}
    call('/api/v1/knowledge-bases/' + kb['id'], 'PUT', {'name': kb['name'], 'config': {'indexing_strategy': graph_config, 'extract_config': {'enabled': True}}})
    label = 'ENTITY' + kb['id'].replace('-', '_')
    graph_query(dep, f"CREATE (a:`{label}` {{name:'Synthetic service',kg:'{document['id']}',attributes:['synthetic only']}})-[:DEPENDS_ON]->(b:`{label}` {{name:'Synthetic worker',kg:'{document['id']}'}})")
    path = '/api/v1/mindcreek/graph/knowledge-bases/' + kb['id']
    data = call(path)['data']
    check('current_graph_browser_real_neo4j', len(data['nodes']) == 2 and len(data['edges']) == 1)
    call(path, who=employee, expected=(403,))
    check('current_graph_viewer_denied')
    agent_config = {'agent_mode': 'quick-answer', 'kb_selection_mode': 'selected', 'knowledge_bases': [kb['id']],
                    'memory_enabled': False, 'mcp_selection_mode': 'none', 'mcp_services': [],
                    'skills_selection_mode': 'none', 'selected_skills': [], 'sandbox_config_id': '', 'web_search_enabled': False}
    agent = current_features(g, call, agent_config)
    from image_resources_smoke import verify as verify_image_resources
    verify_image_resources(g)
    settings = {'name': 'Synthetic portal', 'enabled': True, 'allowed_origins': ['https://portal.synthetic.test'],
                'welcome_message': 'Synthetic only', 'rate_limit_per_minute': 100, 'rate_limit_per_day': 1000, 'allow_file_upload': False}
    prefix = '/api/v1/mindcreek/assistant'
    channel = call(f"{prefix}/agents/{agent['id']}/channels", 'POST', settings, expected=(201,))['data']
    consumer = f"{prefix}/{tenant}/{channel['id']}"
    headers = {'X-MindCreek-Host-Origin': 'https://portal.synthetic.test'}
    call(consumer + '/config', headers=headers, expected=(403,))
    call(consumer + '/config', who=employee, headers=headers)
    session = call(consumer + '/sessions', 'POST', {'title': 'Current package verification'}, who=employee, headers=headers, expected=(201,))['data']['id']
    call(consumer + f'/proxy/api/v1/knowledge-chat/{session}', 'POST', {'query': 'What is the synthetic recovery code?'}, who=employee, headers=headers)
    history = call(consumer + f'/proxy/api/v1/messages/{session}/load', who=employee, headers=headers)['data']
    check('current_r5_employee_channel_question_history', len(history) >= 2)
    call(f'/api/v1/messages/{session}/load', who=employee, expected=(403,))
    check('current_r5_channel_history_not_visible_through_main_api')
    request('/mindcreek-assistant.js')
    frame = f"/assistant/{tenant}/{channel['id']}?host_origin=" + urllib.parse.quote('https://portal.synthetic.test', safe='')
    response = request(frame, expected=(200,))
    check('current_r5_nginx_dynamic_frame_policy', 'https://portal.synthetic.test' in response[1].get('Content-Security-Policy', ''))
    backup = state / 'backup/current-cold'
    meta = ops.backup(dep, backup)
    check('current_cold_backup_includes_all_volumes', {'postgres-data', 'redis-data', 'data-files', 'docreader-tmp', 'mindcreek_graph_data'} <= meta['volumes'].keys())
    recovery = local / ('restore-' + secrets.token_hex(4))
    project = 'mindcreek-current-restore-' + secrets.token_hex(4)
    before = dep.STATE, dep.PROJECT
    before_cmd = g['cmd']
    try:
        dep.STATE, dep.PROJECT = recovery, project
        ops.restore(dep, backup)
        ops.start(dep, meta['running_services'])
        g['cmd'] = [str(v).replace(str(before[0]), str(recovery)).replace(before[1], project) for v in before_cmd]
        status = json.loads(dep.install_command(['status'], True))
        check('current_isolated_restore_preserves_installation', status['stage'] == 'ready' and status['default_tenant_id'] == tenant)
        count = dep.compose(['exec', '-T', 'postgres', 'psql', '-U', 'mindcreek', '-d', 'mindcreek_r4', '-Atc',
            'SELECT count(*) FROM mindcreek.assistant_sessions; SELECT max(version) FROM mindcreek.schema_migrations; '
            'SELECT count(*) FROM mindcreek.kb_policies; SELECT count(*) FROM mindcreek.agent_icons; '
            'SELECT count(*) FROM mindcreek.workspace_admin_delegations; SELECT count(*) FROM mindcreek.kb_execution_delegations'], True).strip().splitlines()
        check('current_isolated_restore_preserves_permissions_icons_and_r5', len(count) == 6 and int(count[0]) >= 1
              and count[1] == str(dep.release()['product_migration_version']) and all(int(n) >= 1 for n in count[2:]))
        check('current_isolated_restore_preserves_graph', '2' in graph_query(dep, f'MATCH (n:`{label}`) RETURN count(n) AS count'))
        features = g['current_feature_facts']
        restored_brand = request('/api/v1/mindcreek/branding')[0]['data']
        request('/api/v1/mindcreek/agent-icons/' + features['avatar'].removeprefix('mc-avatar:') + '?agent_id=' + features['agent_id'], token=token, tenant=tenant)
        check('current_isolated_restore_preserves_branding', restored_brand['name'] == features['brand_name'] and restored_brand['revision'] == features['brand_revision'])
        from image_resources_smoke import verify_restored as verify_restored_images
        verify_restored_images(g)
        call('/api/v1/chunks/' + document['id'], who=employee)
        call('/api/v1/knowledge/' + document['id'] + '/preview', who=employee, expected=(403,))
        call(consumer + f'/proxy/api/v1/messages/{session}/load', who=employee, headers=headers)
        check('current_isolated_restore_real_employee_document_and_history')
    finally:
        try:
            dep.compose(['--profile', 'tls', '--profile', 'tools', 'down', '--volumes', '--remove-orphans'])
        finally:
            dep.STATE, dep.PROJECT = before
            g['cmd'] = before_cmd
    ops.start(dep, meta['running_services'])
    check('current_original_instance_can_resume_after_backup')
