"""Additional checks for the current real AMD64 bundle; synthetic identities only."""
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import urllib.parse


def load(path, name):
    loader=importlib.machinery.SourceFileLoader(name,str(path));spec=importlib.util.spec_from_loader(name,loader)
    module=importlib.util.module_from_spec(spec);loader.exec_module(module);return module


def prepare_old_addons(g):
    """Reproduce the released R4 graph/UI overlays on a disposable old installation."""
    state, root = g['STATE'], g['ROOT']
    graph=load(root/'deploy/graph/graph.py','old_graph_fixture')
    directory=state/'graph';directory.mkdir(mode=0o711)
    password=secrets.token_hex(24);(directory/'password').write_text(password+'\n');(directory/'password').chmod(0o600)
    current=json.loads((state/'compose.override.json').read_text())
    updated=graph.configure(current,state,password,True)
    # Old installer does not include Neo4j in CORE; dependencies make it start privately.
    updated['services']['app']['depends_on']={'neo4j':{'condition':'service_healthy'}}
    links=state/'ui-links';links.mkdir();(links/'links.json').write_text('{"version":1,"enabled":false,"links":{}}\n')
    updated['services']['frontend']={'image':'mindcreek-ui:r4-20260918-links1','volumes':[str(links)+':/usr/share/nginx/html/mindcreek-runtime:ro']}
    (state/'compose.override.json').write_text(json.dumps(updated));(state/'compose.override.json').chmod(0o600)
    caps=json.loads((g['PACKAGE']/'config/r3-capabilities.json').read_text());caps['capabilities']['rag_graph']=True
    (directory/'capabilities.json').write_text(json.dumps(caps))
    (directory/'routes.json').write_bytes((root/'images/archives/mindcreek-r4-20260916-graph1-amd64-addon/routes.json').read_bytes())


def additional_checks(g):
    dep, request, check = g['dep'], g['request'], g['check']
    root, state, local = g['ROOT'], g['STATE'], g['LOCAL']
    token, employee, tenant, kb, document = g['token'], g['employee_token'], g['tenant'], g['kb'], g['upload']
    target=Path((root/'.local/enterprise-current/package-path').read_text().strip())
    ops=load(target/'bin/release','current_release_ops')
    if os.environ.get('MINDCREEK_UPGRADE_SMOKE') == 'true':
        old=dep;dep=load(target/'bin/mindcreek','upgrade_target')
        fingerprints={name:ops.sha(state/name) for name in ('enterprise.env','secrets/admin-password','graph/password','ui-links/links.json')}
        proposed=ops.plan(old.ROOT,dep)[2]
        check('upgrade_recognizes_old_graph_and_ui_pins',len(proposed['changes'])>=4)
        ops.upgrade(dep,old.ROOT,state/'backup/before-upgrade')
        check('upgrade_preserves_identity_models_password_graph_and_links',all(ops.sha(state/name)==digest for name,digest in fingerprints.items()))
        g['dep']=dep;g['PACKAGE']=target
        g['cmd']=[str(x).replace(str(old.ROOT),str(target)) for x in g['cmd']]
        g['report']['release']=dep.release()['release'];g['report']['package_runtime_sha256']=dep.release()['package_runtime_sha256']
        g['report']['upgrade_from']=old.release()['release']
        g['report']['source_images']=g['report']['images'];g['report']['images']=dep.release()['images']
        request('/api/v1/auth/me',token=token,tenant=tenant)
        request('/api/v1/knowledge/'+document['id']+'/preview',token=employee,tenant=tenant)
        check('upgrade_old_admin_employee_sessions_and_document_survive')
    # Enable the shipped optional graph runtime, keeping all existing identity/model values.
    ops.graph(dep, True)
    values=dep.read_env();values['MINDCREEK_EMPLOYEE_ASSISTANT_ENABLED']='true'
    (state/'enterprise.env').write_text(''.join(k+'='+v+'\n' for k,v in values.items()))
    dep.validate();dep.up_services(['neo4j','app','gateway'])
    check('current_optional_neo4j_apoc_healthy')
    def call(path,method='GET',body=None,who=token,headers=None,expected=(200,)):
        return request(path,method,body,token=who,tenant=tenant,headers=headers,expected=expected)[0]
    graph_config={**kb['indexing_strategy'],'graph_enabled':True}
    call('/api/v1/knowledge-bases/'+kb['id'],'PUT',{'name':kb['name'],'config':{'indexing_strategy':graph_config,'extract_config':{'enabled':True}}})
    label='ENTITY'+kb['id'].replace('-','_')
    sql=f"CREATE (a:`{label}` {{name:'Synthetic service',kg:'{document['id']}',attributes:['synthetic only']}})-[:DEPENDS_ON]->(b:`{label}` {{name:'Synthetic worker',kg:'{document['id']}'}})"
    dep.compose(['exec','-T','neo4j','sh','-c','NEO4J_USERNAME=neo4j NEO4J_PASSWORD="${NEO4J_AUTH#neo4j/}" cypher-shell -a bolt://localhost:7687 "$1"','sh',sql],True)
    path='/api/v1/mindcreek/graph/knowledge-bases/'+kb['id']
    data=call(path)['data'];check('current_graph_browser_real_neo4j',len(data['nodes'])==2 and len(data['edges'])==1)
    call(path,who=employee,expected=(403,));check('current_graph_viewer_denied')
    agent=call('/api/v1/agents','POST',{'name':'Current package assistant','config':{'agent_mode':'quick-answer','kb_selection_mode':'selected','knowledge_bases':[kb['id']]}},expected=(200,201))['data']
    settings={'name':'Synthetic portal','enabled':True,'allowed_origins':['https://portal.synthetic.test'],
              'welcome_message':'Synthetic only','rate_limit_per_minute':100,'rate_limit_per_day':1000,'allow_file_upload':False}
    prefix='/api/v1/mindcreek/assistant'
    channel=call(f"{prefix}/agents/{agent['id']}/channels",'POST',settings,expected=(201,))['data']
    consumer=f"{prefix}/{tenant}/{channel['id']}";headers={'X-MindCreek-Host-Origin':'https://portal.synthetic.test'}
    call(consumer+'/config',headers=headers,expected=(403,))
    call(consumer+'/config',who=employee,headers=headers)
    session=call(consumer+'/sessions','POST',{'title':'Current package verification'},who=employee,headers=headers,expected=(201,))['data']['id']
    call(consumer+f'/proxy/api/v1/knowledge-chat/{session}','POST',{'query':'What is the synthetic recovery code?'},who=employee,headers=headers)
    history=call(consumer+f'/proxy/api/v1/messages/{session}/load',who=employee,headers=headers)['data']
    check('current_r5_employee_channel_question_history',len(history)>=2)
    call(f'/api/v1/messages/{session}/load',who=employee,expected=(403,))
    check('current_r5_channel_history_not_visible_through_main_api')
    request('/mindcreek-assistant.js')
    frame=f"/assistant/{tenant}/{channel['id']}?host_origin="+urllib.parse.quote('https://portal.synthetic.test',safe='')
    response=request(frame,expected=(200,))
    check('current_r5_nginx_dynamic_frame_policy','https://portal.synthetic.test' in response[1].get('Content-Security-Policy',''))
    backup=state/'backup/current-cold'
    meta=ops.backup(dep,backup)
    check('current_cold_backup_includes_all_volumes',{'postgres-data','redis-data','data-files','docreader-tmp','mindcreek_graph_data'} <= meta['volumes'].keys())
    recovery=local/('restore-'+secrets.token_hex(4));project='mindcreek-current-restore-'+secrets.token_hex(4)
    before=(dep.STATE,dep.PROJECT)
    before_cmd=g['cmd']
    try:
        dep.STATE=recovery;dep.PROJECT=project
        ops.restore(dep,backup)
        # The restored source ports are safe while the original is still stopped.
        ops.start(dep,meta['running_services'])
        g['cmd']=[str(v).replace(str(before[0]),str(recovery)).replace(before[1],project) for v in before_cmd]
        status=json.loads(dep.install_command(['status'],True))
        check('current_isolated_restore_preserves_installation',status['stage']=='ready' and status['default_tenant_id']==tenant)
        count=dep.compose(['exec','-T','postgres','psql','-U','mindcreek','-d','mindcreek_r4','-Atc','SELECT count(*) FROM mindcreek.assistant_sessions'],True).strip()
        check('current_isolated_restore_preserves_r5_bindings',int(count)>=1)
        output=dep.compose(['exec','-T','neo4j','sh','-c','NEO4J_USERNAME=neo4j NEO4J_PASSWORD="${NEO4J_AUTH#neo4j/}" cypher-shell -a bolt://localhost:7687 "$1"','sh',f'MATCH (n:`{label}`) RETURN count(n) AS count'],True)
        check('current_isolated_restore_preserves_graph','2' in output)
        call('/api/v1/knowledge/'+document['id']+'/preview',who=employee)
        call(consumer+f'/proxy/api/v1/messages/{session}/load',who=employee,headers=headers)
        check('current_isolated_restore_real_employee_document_and_history')
    finally:
        try: dep.compose(['--profile','tls','--profile','tools','down','--volumes','--remove-orphans'])
        finally: dep.STATE,dep.PROJECT=before;g['cmd']=before_cmd
    ops.start(dep,meta['running_services'])
    check('current_original_instance_can_resume_after_backup')
