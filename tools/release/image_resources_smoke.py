"""Actual image proxy, historical repair and revocation checks on synthetic data."""
import base64
import hashlib
import json
from pathlib import Path
import secrets
import subprocess
import sys
import urllib.parse
import uuid


CHECKS = {
    'image_historical_parser_provenance_restores_owner_and_visible_member',
    'image_unverified_manager_read_and_member_fail_closed',
    'image_operator_audit_review_apply_restores_legacy',
    'image_private_creator_and_shared_contributor',
    'image_admin_reads_space_images_and_demotion_rechecks',
    'image_forged_visible_kb_reference_denied',
    'image_answer_proxy_and_session_ownership',
    'image_visibility_revocation_blocks_document_and_answer',
    'image_isolated_restore_preserves_reviewed_bindings_and_png',
}


def quoted(value):
    return "'" + str(value).replace("'", "''") + "'"


def sql(g, statement):
    return g['dep'].compose(['exec', '-T', 'postgres', 'psql', '-U', 'mindcreek', '-d', 'mindcreek_r4',
                             '-v', 'ON_ERROR_STOP=1', '-Atc', statement], True)


def seed_resource(g, knowledge_id, provenance=True):
    from current_smoke import png_fixture
    tenant = int(g['tenant'])
    identifier = str(uuid.uuid4())
    handle = secrets.token_urlsafe(16)
    filename = 'synthetic-image-' + secrets.token_hex(8) + '.png'
    relative = str(tenant) + '/exports/' + filename
    physical = 'local://' + relative
    png = png_fixture()
    script = ('import base64,pathlib,sys; p=pathlib.Path("/data/files")/sys.argv[1]; '
              'p.parent.mkdir(parents=True,exist_ok=True); p.write_bytes(base64.b64decode(sys.argv[2])); p.chmod(0o644)')
    g['dep'].compose(['run', '--rm', '--no-deps', '--pull', 'never', 'ops', '-c', script,
                      relative, base64.b64encode(png).decode()])
    sql(g, "INSERT INTO public.resources (id,handle,tenant_id,provider,physical_path,location_hash,kind,mime_type,size,state) VALUES (" +
        ','.join([quoted(identifier), quoted(handle), str(tenant), "'local'", quoted(physical),
                  quoted(hashlib.sha256(physical.encode()).hexdigest()), "'image'", "'image/png'", str(len(png)), "'active'"]) + ')')
    reference = 'resource://' + handle
    if provenance:
        parent = 'image-smoke-' + secrets.token_hex(8)
        child = parent + '-image'
        sql(g, "INSERT INTO public.knowledge_processing_spans (knowledge_id,attempt,span_id,parent_span_id,name,kind,status,input) VALUES " +
            '(' + ','.join([quoted(knowledge_id), '1000', quoted(parent), 'NULL', "'multimodal'", "'stage'", "'done'", "'{}'::jsonb"]) + '),' +
            '(' + ','.join([quoted(knowledge_id), '1000', quoted(child), quoted(parent), "'multimodal.image[0]'", "'generation'",
                            "'done'", quoted(json.dumps({'image_url': reference})) + '::jsonb']) + ')')
    return {'reference': reference, 'knowledge_id': knowledge_id, 'resource_id': identifier,
            'png_sha256': hashlib.sha256(png).hexdigest()}


def image_path(kb, reference):
    return '/api/v1/knowledge-bases/' + kb + '/files?' + urllib.parse.urlencode({'file_path': reference})


def prepare_history(g):
    """Create the same historical records before the real upgrade/cold backup."""
    if 'image_resource_seed' in g:
        return
    bound = seed_resource(g, g['upload']['id'])
    unverified = seed_resource(g, g['upload']['id'], provenance=False)
    g['image_resource_seed'] = {'bound': bound, 'unverified': unverified}
    if g['dep'].release()['release'] == 'current-20261019-d3fafdaf':
        response = g['request'](image_path(g['kb']['id'], bound['reference']), token=g['token'], tenant=g['tenant'],
                                expected=(409,))[0]
        g['check']('image_source_1019_reproduces_binding_failure',
                   response.get('code') == 'kb_acl.resource_binding_unavailable')


def check_image(g, path, token, fixture):
    body, headers = g['request'](path, token=token, tenant=g['tenant'])
    if body.get('sha256') != fixture['png_sha256'] or headers.get('Content-Type') != 'image/png':
        raise AssertionError('Image proxy did not return the seeded PNG bytes')
    if 'no-store' not in headers.get('Cache-Control', ''):
        raise AssertionError('Authorized images must not retain stale browser cache after revocation')


def verify(g):
    prepare_history(g)
    request, check, tenant = g['request'], g['check'], g['tenant']
    owner, employee, kb = g['token'], g['employee_token'], g['kb']['id']
    bound, unverified = (g['image_resource_seed'][key] for key in ('bound', 'unverified'))
    bound_path = image_path(kb, bound['reference'])
    check_image(g, bound_path, owner, bound)
    check_image(g, bound_path, employee, bound)
    check('image_historical_parser_provenance_restores_owner_and_visible_member')
    unverified_path = image_path(kb, unverified['reference'])
    check_image(g, unverified_path, owner, unverified)
    request(unverified_path, token=employee, tenant=tenant, expected=(409,))
    check('image_unverified_manager_read_and_member_fail_closed')

    wrapper = Path(g['PACKAGE']) / 'bin/image-resources'
    audit_run = subprocess.run([sys.executable, str(wrapper), 'audit', '--tenant', str(tenant)],
                               check=True, capture_output=True, text=True)
    audit = json.loads(audit_run.stdout)
    candidate = next(item for item in audit['items'] if item['reference'] == unverified['reference'])
    if candidate['owners'] or candidate['reason'] != 'no_trusted_knowledge_provenance':
        raise AssertionError('Historical audit silently inferred an owner')
    manifest = Path(g['STATE']) / 'reviewed-synthetic-image.json'
    manifest.write_text(json.dumps({'tenant_id': int(tenant), 'items': [{
        'reference': candidate['reference'], 'knowledge_id': unverified['knowledge_id'],
        'fingerprint': candidate['fingerprint'], 'reason': 'Synthetic fixture ownership reviewed at creation by release test',
    }]}))
    manifest.chmod(0o600)
    subprocess.run([sys.executable, str(wrapper), 'apply', '--tenant', str(tenant), '--confirm-reviewed',
                    '--manifest', str(manifest)], check=True, capture_output=True, text=True)
    check_image(g, unverified_path, employee, unverified)
    check('image_operator_audit_review_apply_restores_legacy')

    def call(path, method='GET', body=None, who=owner, expected=(200,)):
        return request(path, method, body, token=who, tenant=tenant, expected=expected)[0]

    employee_id = call('/api/v1/auth/me', who=employee)['data']['user']['id']
    role_route = f'/api/v1/tenants/{tenant}/members/'
    call(role_route + employee_id, 'PUT', {'role': 'contributor'})
    other = g['oauth']('image-other-contributor')['token']
    request('/api/v1/mindcreek/onboarding', 'POST', {}, token=other)
    other_id = call('/api/v1/auth/me', who=other)['data']['user']['id']
    call(role_route + other_id, 'PUT', {'role': 'contributor'})
    private = call('/api/v1/knowledge-bases', 'POST', {'name': 'Synthetic private image library', 'type': 'document',
        'indexing_strategy': {'vector_enabled': True, 'keyword_enabled': False, 'graph_enabled': False, 'wiki_enabled': False}},
        who=employee, expected=(200, 201))['data']
    document = call(f"/api/v1/knowledge-bases/{private['id']}/knowledge/manual", 'POST',
        {'title': 'Synthetic image source', 'content': 'Synthetic private image recovery guide.', 'status': 'publish'},
        who=employee, expected=(200, 201))['data']
    if g['processing'](document['id'], employee, tenant)['parse_status'] != 'completed':
        raise AssertionError('Synthetic image source did not process')
    resource = seed_resource(g, document['id'])
    private_path = image_path(private['id'], resource['reference'])
    check_image(g, private_path, owner, resource)
    check_image(g, private_path, employee, resource)
    request(private_path, token=other, tenant=tenant, expected=(403, 404))
    call(role_route + other_id, 'PUT', {'role': 'admin'})
    check_image(g, private_path, other, resource)
    check_image(g, unverified_path, other, unverified)
    call(role_route + other_id, 'PUT', {'role': 'contributor'})
    request(private_path, token=other, tenant=tenant, expected=(403, 404))
    check('image_admin_reads_space_images_and_demotion_rechecks')
    request(image_path(kb, resource['reference']), token=other, tenant=tenant, expected=(404,))
    # A pasted native lifecycle claim must not turn a private resource into a public one.
    sql(g, "INSERT INTO public.resource_bindings (id,resource_id,tenant_id,owner_type,owner_id,relation) VALUES (" +
        ','.join([quoted(str(uuid.uuid4())), quoted(resource['resource_id']), str(tenant), "'knowledge'",
                  quoted(g['upload']['id']), "'attachment'"]) + ')')
    request(image_path(kb, resource['reference']), token=other, tenant=tenant, expected=(404,))
    check('image_forged_visible_kb_reference_denied')
    policy_path = '/api/v1/mindcreek/kb-permissions/' + private['id']
    policy = call(policy_path, who=employee)['data']
    call(policy_path, 'PUT', {'revision': policy['revision'], 'visibility': 'restricted',
        'members': [{'user_id': other_id, 'visible': True, 'operations': []}]}, who=employee)
    check_image(g, private_path, other, resource)
    check('image_private_creator_and_shared_contributor')

    session = call('/api/v1/sessions', 'POST', {'title': 'Synthetic historical image answer'}, who=other, expected=(200, 201))['data']['id']
    call('/api/v1/knowledge-chat/' + session, 'POST',
         {'query': 'What is the synthetic image recovery guide?', 'knowledge_base_ids': [private['id']], 'disable_title': True}, who=other)
    history = call('/api/v1/messages/' + session + '/load', who=other)['data']
    message = next(item for item in history if item.get('role') == 'assistant')
    # Model prose is synthetic. Seed an old answer image to exercise the exact
    # persisted-message read route without making claims about real model quality.
    sql(g, 'UPDATE public.messages SET content=' + quoted('Synthetic historical answer ![image](' + resource['reference'] + ')') +
        ',agent_tenant_id=' + str(tenant) + ' WHERE id=' + quoted(message['id']) + ' AND session_id=' + quoted(session))
    history = call('/api/v1/messages/' + session + '/load', who=other)['data']
    if resource['reference'] not in json.dumps(history):
        raise AssertionError('Historical image reference disappeared from readable answer')
    message_path = '/api/v1/sessions/' + session + '/messages/' + message['id'] + '/files?' + urllib.parse.urlencode({'file_path': resource['reference']})
    check_image(g, message_path, other, resource)
    request(message_path, token=employee, tenant=tenant, expected=(403, 404))
    check('image_answer_proxy_and_session_ownership')
    policy = call(policy_path, who=employee)['data']
    call(policy_path, 'PUT', {'revision': policy['revision'], 'visibility': 'restricted', 'members': []}, who=employee)
    request(private_path, token=other, tenant=tenant, expected=(403, 404))
    request(message_path, token=other, tenant=tenant, expected=(403, 404))
    check('image_visibility_revocation_blocks_document_and_answer')
    call(role_route + employee_id, 'PUT', {'role': 'viewer'})
    call(role_route + other_id, 'PUT', {'role': 'viewer'})


def verify_restored(g):
    fixtures = g['image_resource_seed']
    for item in fixtures.values():
        check_image(g, image_path(g['kb']['id'], item['reference']), g['employee_token'], item)
    count = sql(g, 'SELECT count(*) FROM mindcreek.kb_resource_bindings WHERE tenant_id=' + str(g['tenant']) +
        ' AND resource_id=' + quoted(fixtures['unverified']['resource_id'])).strip()
    g['check']('image_isolated_restore_preserves_reviewed_bindings_and_png', count == '1')
