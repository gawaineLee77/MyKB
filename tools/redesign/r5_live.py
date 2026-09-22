"""Employee channel acceptance on actual native API/DB and nginx; synthetic data only."""
import json
import os
from pathlib import Path
import subprocess
import time
import base64
import struct
import zlib
from urllib.parse import quote
from concurrent.futures import ThreadPoolExecutor

PUBLIC = 'http://mindcreek.localhost:18685'
PORTAL = 'http://localhost:18687'

def configure(root, run, services, environment, gateway, report):
    ui=Path((root/'.local/redesign-r5/ui-path').read_text().strip())
    gateway.update({'MINDCREEK_EMPLOYEE_ASSISTANT_ENABLED':'true','MINDCREEK_EXTERNAL_ORIGIN':PUBLIC,'MINDCREEK_IDENTITY_AUTHORIZATION_URL':'http://localhost:18686/authorize'})
    environment['OIDC_AUTH_ISSUER_URL']=PUBLIC+'/api/v1/mindcreek/oidc'
    environment['SSRF_WHITELIST_EXTRA'] += ',mindcreek.localhost' 
    services['identity']['volumes']=[str(root/'testdata/redesign/r5_identity.py')+':/fixture/mock_identity.py:ro']
    services['identity']['ports']=['127.0.0.1:18686:18000']
    nginx_image='nginx:1.30.3-alpine'
    nginx_id=subprocess.check_output(['docker','image','inspect',nginx_image,'--format','{{.Id}}'],text=True).strip()
    report['images']['nginx']={'tag':nginx_image,'id':nginx_id}
    conf=(root/'deploy/enterprise/nginx.frontend.conf').read_text()
    for key,value in {'APP_SCHEME':'http','APP_HOST':'gateway','APP_PORT':'8080','MAX_FILE_SIZE':'50m','MAX_SKILL_BUNDLE_SIZE':'50m'}.items():conf=conf.replace('${'+key+'}',value)
    conf=conf.replace('listen 80;','listen 18685;')
    (run/'frontend.conf').write_text(conf)
    services['frontend']={'image':nginx_id,'ports':['127.0.0.1:18685:18685'],'networks':{'default':{'aliases':['mindcreek.localhost']}},'volumes':[str(ui/'dist')+':/usr/share/nginx/html:ro',str(run/'frontend.conf')+':/etc/nginx/conf.d/default.conf:ro',str(root/'upstream/weknora/frontend/nginx-api-proxy.conf')+':/etc/nginx/api-proxy.conf:ro'],'depends_on':['gateway']}
    (run/'portal').mkdir()
    services['portal']={'image':nginx_id,'ports':['127.0.0.1:18687:80'],'volumes':[str(run/'portal')+':/usr/share/nginx/html:ro']}
    before_image='mindcreek-ui:r4-20260918-links1'
    before_id=subprocess.check_output(['docker','image','inspect',before_image,'--format','{{.Id}}'],text=True).strip()
    report['images']['before_ui']={'tag':before_image,'id':before_id}
    services['before']={'image':before_id,'ports':['127.0.0.1:18688:80'],'environment':{'APP_HOST':'gateway','APP_PORT':'8080','APP_SCHEME':'http'},'depends_on':['gateway']}
    # Docker does not publish ports from an internal-only network. Only the
    # browser-facing services join this additional bridge; DB/app stay private.
    for name in ('frontend','portal','identity','before'):
        services[name].setdefault('networks',{'default':{}})['browser']={}
    report['scope']='R5 actual native v0.8.0, gateway, PostgreSQL, nginx, cross-origin browser and synthetic browser OAuth/model services; no corporate deployment acceptance'

def assistant_checks(root, run, request, check, oauth, docker, tenant, other, admin, password, base):
    prefix='/api/v1/mindcreek/assistant'
    def call(path,method='GET',body=None,token=admin,space=tenant,expected=(200,),headers=None):
        return request(path,method,body,token=token,tenant=space,expected=expected,headers=headers)[0]
    actors={}
    for name in ('alice','bob','manager'):
        actor=oauth('r5-browser-'+name)
        call('/api/v1/mindcreek/onboarding','POST',{},token=actor['token'])
        user=call('/api/v1/auth/me',token=actor['token'])['data']['user']
        actors[name]={'token':actor['token'],'id':user['id']}
    call(f'/api/v1/tenants/{tenant}/members/{actors["manager"]["id"]}','PUT',{'role':'admin'})
    kb=call('/api/v1/knowledge-bases','POST',{'name':'R5 合成员工手册','type':'document'},expected=(200,201))['data']
    document=call(f'/api/v1/knowledge-bases/{kb["id"]}/knowledge/manual','POST',{'title':'R5 操作指南','content':'# Synthetic employee guide\n\nThe synthetic recovery code is R5-VERIFY. Restart the worker and check its health.','status':'publish'})['data']
    for _ in range(60):
        parsed=call(f'/api/v1/knowledge/{document["id"]}')['data']
        if parsed['parse_status']=='completed':break
        if parsed['parse_status']=='failed':raise RuntimeError('Synthetic manual failed')
        time.sleep(1)
    check('r5.native_document_processed',parsed['parse_status']=='completed')
    agent=call('/api/v1/agents','POST',{'name':'企业知识小助手 · 合成验证','config':{'agent_mode':'quick-answer','kb_selection_mode':'selected','knowledge_bases':[kb['id']],'image_upload_enabled':True}},expected=(200,201))['data']
    settings={'name':'员工门户助手','enabled':True,'allowed_origins':[PORTAL,PUBLIC],'welcome_message':'你好，我可以帮你查询员工手册和操作指南。','rate_limit_per_minute':100,'rate_limit_per_day':10000,'allow_file_upload':True}
    path=f'{prefix}/agents/{agent["id"]}/channels'
    channel=call(path,'POST',settings,expected=(201,))['data']
    check('r5.channel.owner_create_no_secret','publish_token' not in channel and 'webhook_secret' not in channel)
    call(path,token=actors['manager']['token'])
    call(f'{prefix}/channels/{channel["id"]}','PUT',settings,token=actors['manager']['token'])
    for role in ('contributor','viewer'):
        call(f'/api/v1/tenants/{tenant}/members/{actors["bob"]["id"]}','PUT',{'role':role})
        actors['bob']['token']=oauth('r5-browser-bob')['token']
        call(path,'POST',settings,token=actors['bob']['token'],expected=(403,))
    check('r5.channel.owner_admin_only')
    consumer=f'{prefix}/{tenant}/{channel["id"]}'
    head={'X-MindCreek-Host-Origin':PORTAL}
    def employee(path='',method='GET',body=None,who='alice',expected=(200,),headers=None):
        return call(consumer+path,method,body,token=actors[who]['token'],expected=expected,headers={**head,**(headers or {})})
    call(consumer+'/config',headers=head,expected=(403,))
    call(consumer+'/config',token='',headers=head,expected=(401,))
    call(consumer+'/config',token=actors['alice']['token'],space=other,headers=head,expected=(403,))
    check('r5.anonymous_local_admin_and_cross_space_denied')
    employee('/config')
    session=employee('/sessions','POST',{'title':'API 验证'},expected=(201,))['data']['id']
    proxy='/proxy/api/v1/'
    employee(proxy+f'knowledge-chat/{session}','POST',{'query':'What is the recovery code?'})
    history=employee(proxy+f'messages/{session}/load')['data']
    check('r5.employee_native_chat_and_history',len(history)>=2)
    refs=[ref for m in history for ref in (m.get('knowledge_references') or [])]
    chunk_id=next((r.get('chunk_id') or r.get('id') for r in refs if r.get('chunk_id') or r.get('id')),None)
    if not chunk_id:
        chunk_id=docker('exec','-T','postgres','psql','-U','r1','-d','r1','-Atc',f"SELECT id FROM chunks WHERE knowledge_id='{document['id']}' AND chunk_type='text' LIMIT 1")
    check('r5.authorized_citation',bool(employee(proxy+f'chunks/by-id/{chunk_id}',headers={'X-MindCreek-Session-ID':session})['data']['content']))
    # Construct a tiny synthetic PNG; prove persisted images use employee files.
    def block(kind, data): return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data))
    png=b'\x89PNG\r\n\x1a\n'+block(b'IHDR',struct.pack('>IIBBBBB',2,2,8,2,0,0,0))+block(b'IDAT',zlib.compress(b'\x00'+b'\x20\x80\x60'*2+b'\x00'+b'\x20\x80\x60'*2))+block(b'IEND',b'')
    (run/'tiny.png').write_bytes(png)
    employee(proxy+f'knowledge-chat/{session}','POST',{'query':'Read this synthetic note','images':[{'data':'data:image/png;base64,'+base64.b64encode(png).decode()}],'attachment_uploads':[{'data':'data:text/plain;base64,'+base64.b64encode(b'Synthetic R5 attachment.').decode(),'file_name':'synthetic.txt','file_size':24}]})
    media_history=employee(proxy+f'messages/{session}/load')['data']
    user=next(m for m in media_history if m.get('role')=='user' and m.get('images'))
    employee(proxy+f'sessions/{session}/messages/{user["id"]}/files?file_path='+quote(user['images'][0]['url'],safe=''))
    check('r5.inline_upload_and_employee_image_file')
    employee(proxy+f'messages/{session}/load',who='bob',expected=(403,))
    check('r5.two_employee_session_isolation')
    before=request('http://models:19090/counts')[0]
    for suffix,body in [('',{'query':'deny','knowledge_base_ids':['forbidden']}),('?agent_id=forbidden',{'query':'deny'}),('?resource_urls=public',{'query':'deny'})]:
        employee(proxy+f'knowledge-chat/{session}'+suffix,'POST',body,expected=(400,))
    employee(proxy+f'knowledge-chat/{session}','POST',{'query':'deny'},headers={'X-MindCreek-Host-Origin':'https://evil.example'},expected=(403,))
    call(f'/api/v1/messages/{session}/load',token=actors['alice']['token'],expected=(403,))
    check('r5.denials_before_model_and_no_main_site_bypass',request('http://models:19090/counts')[0]==before)
    call(f'/api/v1/agents/{agent["id"]}','PUT',{'name':agent['name'],'config':{**agent['config'],'kb_selection_mode':'none','knowledge_bases':[]}})
    employee(proxy+f'messages/{session}/load',expected=(403,))
    call(f'/api/v1/agents/{agent["id"]}','PUT',{'name':agent['name'],'config':agent['config']})
    check('r5.agent_scope_shrink_revokes_history')
    subject=docker('exec','-T','postgres','psql','-U','r1','-d','r1','-Atc',f"SELECT broker_subject FROM mindcreek.corporate_identities WHERE local_user_id='{actors['alice']['id']}'")
    call(f'/api/v1/mindcreek/identities/{subject}/suspend','POST',{})
    employee('/config',expected=(401,403))
    call(f'/api/v1/mindcreek/identities/{subject}/activate','POST',{})
    actors['alice']['token']=oauth('r5-browser-alice')['token']
    check('r5.suspended_employee_denied')
    settings['enabled']=False
    call(f'{prefix}/channels/{channel["id"]}','PUT',settings)
    for suffix in (proxy+f'messages/{session}/load',proxy+f'sessions/continue-stream/{session}?message_id=synthetic',proxy+f'chunks/by-id/synthetic'):
        employee(suffix,expected=(403,))
    check('r5.channel_disable_revokes_history_reconnect_citation')
    settings['enabled']=True;call(f'{prefix}/channels/{channel["id"]}','PUT',settings)
    # Rate counters are in PostgreSQL, atomically shared by competing requests.
    limited={**settings,'name':'Limited','rate_limit_per_minute':2}
    limit_channel=call(path,'POST',limited,expected=(201,))['data']
    def one(_):return request(f'{prefix}/{tenant}/{limit_channel["id"]}/sessions','POST',{'title':'Concurrent'},token=actors['alice']['token'],tenant=tenant,headers=head,expected=(201,429))[2]
    with ThreadPoolExecutor(max_workers=6) as pool: statuses=list(pool.map(one,range(6)))
    check('r5.concurrent_rate_budget',statuses.count(201)==2 and statuses.count(429)==4)
    docker('restart','gateway')
    for _ in range(30):
        try: employee('/config');break
        except (OSError,AssertionError):time.sleep(1)
    request(f'{prefix}/{tenant}/{limit_channel["id"]}/sessions','POST',{},token=actors['alice']['token'],tenant=tenant,headers=head,expected=(429,))
    employee(proxy+f'messages/{session}/load')
    check('r5.restart_preserves_bindings_and_budget')
    call('/api/v1/auth/logout','POST',{},token=actors['bob']['token'])
    employee('/config',who='bob',expected=(401,))
    actors['bob']['token']=oauth('r5-browser-bob')['token']
    call(f'/api/v1/tenants/{tenant}/members/{actors["bob"]["id"]}','DELETE')
    employee('/config',who='bob',expected=(401,403))
    check('r5.employee_logout_and_removal_revoked')
    call(f'/api/v1/tenants/{tenant}/members','POST',{'email':'r5-browser-bob@example.invalid','role':'viewer'},expected=(201,))
    # A real browser popup now traverses the synthetic IdP; no API interception.
    portal='''<!doctype html><html lang="zh"><meta charset="utf-8"><title>员工工作台 · 合成验收</title><style>body{font:16px system-ui;margin:0;background:#f4f7f6;color:#1d3830}header{background:white;padding:24px 48px;border-bottom:1px solid #dce6e0}main{margin:60px;max-width:760px}h1{font-size:38px}section{background:white;padding:28px;border-radius:14px;margin:20px 0}small{color:#698078}</style><header>MindCreek / 员工工作台</header><main><small>R5 · 独立来源网页 · 合成内容</small><h1>让知识伴随日常工作</h1><p>在当前网页中查询员工手册和操作指南。</p><section><h2>工作通知</h2><p>本页面用于员工小助手的合成验收。</p></section></main>'''
    (run/'portal/index.html').write_text(portal+f'<script src="{PUBLIC}/mindcreek-assistant.js" data-space="{tenant}" data-channel="{channel["id"]}" defer></script></html>')
    fixture=run/'browser-secrets.json';fixture.write_text(json.dumps({'tenant':tenant,'channel':channel['id'],'agent':agent['id'],'admin':admin,'manager':actors['manager']['token'],'settings':settings,'session':session,'image':str(run/'tiny.png')}));fixture.chmod(0o600)
    env={**os.environ,'R5_BROWSER_SECRETS':str(fixture),'R5_ROOT':str(root)}
    try:
        with (root/'.local/redesign-r5/browser.log').open('w') as log:
            subprocess.run([env.get('R5_NODE_BIN','node'),str(root/'tools/redesign/r5_browser.mjs')],env=env,stdout=log,stderr=subprocess.STDOUT,check=True)
        check('r5.cross_origin_browser_and_synthetic_oauth')
    finally:fixture.unlink(missing_ok=True)
