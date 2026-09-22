#!/usr/bin/env python3
"""Exercise the shipped AMD64 images/installer in a disposable synthetic project."""
import base64
import datetime
import hashlib
import http.cookies
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.parse

ROOT=Path(__file__).resolve().parents[2]
LOCAL=ROOT/'.local/enterprise-amd64'
PACKAGE=Path((LOCAL/'package-path').read_text().strip())
STATE=Path(tempfile.mkdtemp(prefix='smoke-',dir=LOCAL))
PROJECT='mindcreek-amd64-smoke-'+secrets.token_hex(4)
os.environ.update(MINDCREEK_STATE_DIR=str(STATE),MINDCREEK_PROJECT=PROJECT)
spec=importlib.util.spec_from_file_location('deployment',PACKAGE/'bin/mindcreek',loader=__import__('importlib.machinery').machinery.SourceFileLoader('deployment',str(PACKAGE/'bin/mindcreek')))
dep=importlib.util.module_from_spec(spec);spec.loader.exec_module(dep)
password='Synthetic-'+secrets.token_hex(8)+'!'
dep.initialize(password)
values=dep.read_env()
values.update(MINDCREEK_EXTERNAL_ORIGIN='https://mindcreek.test',MINDCREEK_INSTALL_ADMIN_EMAIL='admin@synthetic.test',MINDCREEK_IDENTITY_ISSUER='https://identity:18000',MINDCREEK_IDENTITY_CLIENT_ID='synthetic-client',MINDCREEK_IDENTITY_CLIENT_SECRET=secrets.token_hex(24),SSRF_WHITELIST_EXTRA='models',MINDCREEK_MANAGED_EMBEDDING_DIMENSION='64',MINDCREEK_MANAGED_ALLOW_HTTP='true',MINDCREEK_IDENTITY_SCOPES='profile email')
for name,path in [('AUTHORIZATION','authorize'),('TOKEN','token'),('USERINFO','userinfo')]: values[f'MINDCREEK_IDENTITY_{name}_URL']='https://identity:18000/'+path
for name,kind in [('LLM','chat'),('EMBEDDING','embedding'),('RERANK','rerank')]:
 values.update({f'MINDCREEK_MANAGED_{name}_NAME':'mindcreek-test-'+kind,f'MINDCREEK_MANAGED_{name}_BASE_URL':'http://models:19090/v1',f'MINDCREEK_MANAGED_{name}_API_KEY':secrets.token_hex(24)})
with socket.socket() as sock:
 sock.bind(('127.0.0.1',0));values['FRONTEND_PORT']=str(sock.getsockname()[1])
with socket.socket() as sock:
 sock.bind(('127.0.0.1',0));values['TLS_PORT']=str(sock.getsockname()[1])
values['TLS_BIND_IP']='127.0.0.1'
(STATE/'enterprise.env').write_text(''.join(k+'='+v+'\n' for k,v in values.items()))
subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','2','-subj','/CN=mindcreek-synthetic','-addext','subjectAltName=DNS:identity,DNS:tls,DNS:mindcreek.test','-keyout',str(STATE/'tls/privkey.pem'),'-out',str(STATE/'tls/fullchain.pem')],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
(STATE/'tls/privkey.pem').chmod(0o600)
shutil.copy2(STATE/'tls/fullchain.pem',STATE/'ca/synthetic.crt')
fixture=STATE/'fixtures';fixture.mkdir()
shutil.copy2(ROOT/'testdata/redesign/mock_identity.py',fixture)
shutil.copy2(ROOT/'tools/phase0/mock_openai.py',fixture)
(fixture/'identity_tls.py').write_text('''import http.server,ssl,runpy
original=http.server.HTTPServer.__init__
def init(self,*args,**kwargs):
 original(self,*args,**kwargs)
 context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
 context.load_cert_chain('/cert/fullchain.pem','/cert/privkey.pem')
 self.socket=context.wrap_socket(self.socket,server_side=True)
http.server.HTTPServer.__init__=init
runpy.run_path('/fixture/mock_identity.py',run_name='__main__')
''')
base={'image':'python:3.12-alpine','platform':'linux/amd64','pull_policy':'never','volumes':[str(fixture)+':/fixture:ro',str(STATE/'tls')+':/cert:ro']}
override={'services':{'identity':{**base,'command':['python','/fixture/identity_tls.py']},'models':{**base,'command':['python','/fixture/mock_openai.py','--host','0.0.0.0']}}}
(STATE/'compose.override.json').write_text(json.dumps(override))
report={'release':dep.release()['release'],'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'project':PROJECT,'platform':'linux/amd64','host_execution':'ARM64 Docker Desktop AMD64 emulation','scope':'Actual packaged installer and runtime; isolated synthetic OAuth HTTPS CA and model endpoints. No real corporate OAuth/model or physical x86 server acceptance.','checks':[],'requests':[],'images':dep.release()['images']}
report['package_runtime_sha256']=dep.release()['package_runtime_sha256']
(LOCAL/'smoke-state').write_text(str(STATE)+'\n')
(LOCAL/'smoke-project').write_text(PROJECT+'\n')
def check(name,condition=True,**facts):
 if not condition: raise AssertionError(name)
 report['checks'].append({'name':name,'passed':True,**facts});print('PASS '+name,flush=True)
transport='''import base64,json,sys,urllib.request,urllib.error,ssl
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args):return None
p=json.load(sys.stdin);body=base64.b64decode(p['raw']) if p.get('raw') else None if p['body'] is None else json.dumps(p['body']).encode()
r=urllib.request.Request(p['url'],data=body,method=p['method'],headers=p['headers'])
context=ssl.create_default_context(cafile='/cert/fullchain.pem')
try:r=urllib.request.build_opener(urllib.request.ProxyHandler({}),NoRedirect(),urllib.request.HTTPSHandler(context=context)).open(r,timeout=150)
except urllib.error.HTTPError as e:r=e
raw=r.read()
try:body=json.loads(raw)
except ValueError:body={'raw':raw.decode(errors='replace')}
print(json.dumps({'status':r.code,'headers':dict(r.headers),'body':body}))
'''
cookies={}
cmd=['docker','compose','-p',PROJECT,'--project-directory',str(PACKAGE),'--env-file','/dev/null','-f',str(PACKAGE/'compose.json'),'-f',str(STATE/'compose.override.json')]
def request(path,method='GET',body=None,token=None,tenant=None,headers=None,expected=(200,),raw=None):
 hdr={'Content-Type':'application/json',**(headers or {})}
 if token:hdr['Authorization']='Bearer '+token
 if tenant:hdr['X-Tenant-ID']=str(tenant)
 if cookies:hdr['Cookie']='; '.join(k+'='+v for k,v in cookies.items())
 url=path.replace('https://mindcreek.test','http://frontend') if path.startswith('http') else 'http://frontend'+path
 result=subprocess.run([*cmd,'exec','-T','identity','python','-c',transport],input=json.dumps({'url':url,'method':method,'body':body,'headers':hdr,'raw':base64.b64encode(raw).decode() if raw else None}),text=True,capture_output=True,env=dep.docker_env(),check=True)
 response=json.loads(result.stdout);status=response['status']
 # Only record paths/status. Never record tokens, raw content or query credentials.
 report['requests'].append({'method':method,'path':urllib.parse.urlsplit(path).path,'status':status})
 if status not in expected:raise AssertionError(f'{method} {urllib.parse.urlsplit(path).path}: {status}; expected {expected}; error={response["body"].get("error", "")}')
 for k,v in http.cookies.SimpleCookie(response['headers'].get('Set-Cookie','')).items():cookies[k]=v.value
 return response['body'],response['headers']
def oauth(subject):
 auth=request('/api/v1/auth/oidc/url?'+urllib.parse.urlencode({'redirect_uri':'https://mindcreek.test/api/v1/auth/oidc/callback'}))[0]
 _,headers=request(auth['authorization_url'],expected=(302,))
 state=urllib.parse.parse_qs(urllib.parse.urlsplit(headers['Location']).query)['state'][0]
 _,headers=request('/api/v1/mindcreek/oidc/callback?'+urllib.parse.urlencode({'state':state,'code':subject}),expected=(302,))
 _,headers=request(headers['Location'],expected=(302,))
 fragment=urllib.parse.parse_qs(urllib.parse.urlsplit(headers['Location']).fragment)
 encoded=fragment['oidc_result'][0]
 return json.loads(base64.urlsafe_b64decode(encoded+'='*(-len(encoded)%4)))
def processing(kid,token,tenant):
 deadline=time.monotonic()+180
 while time.monotonic()<deadline:
  doc=request('/api/v1/knowledge/'+kid,token=token,tenant=tenant)[0]['data']
  if doc['parse_status'] in ('completed','failed'):return doc
  time.sleep(2)
 raise AssertionError('Document processing timeout')
try:
 dep.validate();dep.verify_images();check('configuration_and_8_amd64_image_locks')
 dep.up_services(['identity','models'])
 dep.install(['admin']);first=json.loads(dep.install_command(['status'],True))
 check('private_admin_bootstrap',first['stage']=='account_ready')
 dep.install(['default-space']);ready=json.loads(dep.install_command(['status'],True))
 dep.install(['admin']);dep.install(['default-space']);again=json.loads(dep.install_command(['status'],True))
 check('idempotent_admin_and_default_space',ready['stage']=='ready' and ready['admin_user_id']==again['admin_user_id'] and ready['default_tenant_id']==again['default_tenant_id'])
 dep.up_services(dep.CORE+['frontend','tls'])
 services=[json.loads(line) for line in dep.compose(['ps','--format','json'],True).splitlines() if line.startswith('{')]
 check('six_runtime_services_healthy',all(next(s for s in services if s['Service']==n)['Health']=='healthy' for n in dep.CORE+['frontend']))
 request('/');request('/admin/login');request('/embed/x',expected=(404,));request('https://tls/')
 check('native_ui_admin_entry_retired_embed_and_tls')
 auth=request('/api/v1/mindcreek/admin/auth/login','POST',{'email':values['MINDCREEK_INSTALL_ADMIN_EMAIL'],'password':password})[0]
 token=auth['token'];tenant=ready['default_tenant_id']
 me=request('/api/v1/auth/me',token=token,tenant=tenant)[0]['data']
 check('admin_gateway_login_and_owner_membership',me['user']['id']==ready['admin_user_id'])
 employee=oauth('package-employee');employee_token=employee['token']
 onboarding=request('/api/v1/mindcreek/onboarding','POST',{},token=employee_token)[0]['data']
 check('oauth_over_custom_ca_and_first_viewer_onboarding',onboarding['state']=='ready' and len(onboarding['memberships'])==1 and onboarding['memberships'][0]['role']=='viewer')
 kb=request('/api/v1/knowledge-bases','POST',{'name':'AMD64 synthetic verification','type':'document','indexing_strategy':{'vector_enabled':True,'keyword_enabled':False,'graph_enabled':False,'wiki_enabled':False}},token=token,tenant=tenant,expected=(200,201))[0]['data']
 check('native_kb_managed_model_defaults',kb['embedding_model_id']=='builtin-mindcreek-embedding' and kb['summary_model_id']=='builtin-mindcreek-chat')
 # The default hybrid query aborts in this host's AMD64 emulator (separate evidence).
 # Exercise a native vector-only KB without changing any shipped application defaults.
 check('synthetic_kb_explicit_native_vector_only',kb['indexing_strategy']['vector_enabled'] and not kb['indexing_strategy']['keyword_enabled'])
 text='# Offline package check\n\nThe synthetic recovery code is AMD64-VERIFIED. Restart the synthetic worker and verify health.\n'
 boundary='package-synthetic-boundary'
 payload=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="amd64-synthetic.md"\r\nContent-Type: text/markdown\r\n\r\n{text}\r\n--{boundary}--\r\n').encode()
 upload=request(f"/api/v1/knowledge-bases/{kb['id']}/knowledge/file",'POST',token=token,tenant=tenant,headers={'Content-Type':'multipart/form-data; boundary='+boundary},raw=payload)[0]['data']
 parsed=processing(upload['id'],token,tenant)
 check('real_docreader_markdown_upload_and_processing',parsed['parse_status']=='completed')
 # Minimal one-page ASCII PDF: fully synthetic fixture, no outside parser/model assets.
 stream=b'BT /F1 12 Tf 40 750 Td (AMD64 PDF verification. The recovery code is PDF-VERIFIED.) Tj ET'
 objects=[b'<< /Type /Catalog /Pages 2 0 R >>',b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',b'<< /Length '+str(len(stream)).encode()+b' >>\nstream\n'+stream+b'\nendstream']
 pdf=b'%PDF-1.4\n';offsets=[]
 for n,obj in enumerate(objects,1):offsets.append(len(pdf));pdf+=f'{n} 0 obj\n'.encode()+obj+b'\nendobj\n'
 xref=len(pdf);pdf+=b'xref\n0 6\n0000000000 65535 f \n'+b''.join(f'{v:010d} 00000 n \n'.encode() for v in offsets)+f'trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n'.encode()
 payload=f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="amd64-synthetic.pdf"\r\nContent-Type: application/pdf\r\n\r\n'.encode()+pdf+f'\r\n--{boundary}--\r\n'.encode()
 pdf_upload=request(f"/api/v1/knowledge-bases/{kb['id']}/knowledge/file",'POST',token=token,tenant=tenant,headers={'Content-Type':'multipart/form-data; boundary='+boundary},raw=payload)[0]['data']
 check('real_docreader_text_pdf_upload_and_processing',processing(pdf_upload['id'],token,tenant)['parse_status']=='completed')
 faq=request('/api/v1/knowledge-bases','POST',{'name':'AMD64 synthetic FAQ','type':'faq'},token=token,tenant=tenant,expected=(200,201))[0]['data']
 request(f"/api/v1/knowledge-bases/{faq['id']}/faq/entry",'POST',{'standard_question':'What is the synthetic code?','answers':['AMD64-VERIFIED'],'is_enabled':True},token=token,tenant=tenant)
 check('native_faq_create')
 request('/api/v1/knowledge-bases','POST',{'name':'viewer denied'},token=employee_token,tenant=tenant,expected=(403,))
 request(f"/api/v1/knowledge/{upload['id']}/preview",token=employee_token,tenant=tenant)
 request(f"/api/v1/knowledge/{upload['id']}/download",token=employee_token,tenant=tenant,expected=(403,))
 check('viewer_read_and_write_download_boundaries')
 rpc=lambda method,params:request('/mcp','POST',{'jsonrpc':'2.0','id':1,'method':method,'params':params},token=employee_token,tenant=tenant,headers={'MCP-Protocol-Version':'2025-11-25'})[0]
 tools=rpc('tools/list',{})['result']['tools']
 check('four_authenticated_mcp_tools',len(tools)==4)
 answer=rpc('tools/call',{'name':'ask_knowledge_agent','arguments':{'query':'What is the synthetic recovery code?','knowledge_base_ids':[kb['id']]}})
 value=answer.get('result',{}).get('structuredContent',{})
 if not (value.get('answer') and value.get('references')):
  report['mcp_error']=answer.get('error',{'result_keys':list(answer.get('result',{}))})
 check('employee_mcp_vector_only_model_answer_references',bool(value.get('answer')) and bool(value.get('references')))
 request('/api/v1/mindcreek/catalog',token=token,tenant=tenant,expected=(410,));check('legacy_business_retired')
 request('/api/v1/auth/register','POST',{'email':'forbidden@synthetic.test','password':password},expected=(403,404));check('public_password_registration_closed')
 dep.compose(['restart','gateway']);dep.up_services(['gateway'])
 request('/api/v1/auth/me',token=token,tenant=tenant);check('gateway_restart_preserves_admin_session')
 # Use the packaged ops image and mounted data volume, proving backup helper availability.
 dep.compose(['stop','tls','frontend','gateway','app','docreader'])
 with (STATE/'backup/database.dump').open('wb') as out:
  subprocess.run([*cmd,'exec','-T','postgres','pg_dump','-U','mindcreek','-d','mindcreek_r4','-Fc'],stdout=out,env=dep.docker_env(),check=True)
 dep.compose(['exec','-T','redis','sh','-c','REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli SAVE'])
 dep.compose(['stop','redis'])
 dep.compose(['run','--rm','--no-deps','--pull','never','ops','-c','import tarfile; t=tarfile.open("/backup/files-and-redis.tar.gz","w:gz"); t.add("/data/files",arcname="files"); t.add("/redis-data",arcname="redis"); t.close()'])
 check('ops_quiesced_files_and_redis_backup',(STATE/'backup/files-and-redis.tar.gz').stat().st_size>0)
 dep.compose(['exec','-T','postgres','createdb','-U','mindcreek','package_restore'])
 with (STATE/'backup/database.dump').open('rb') as source:
  subprocess.run([*cmd,'exec','-T','postgres','pg_restore','-U','mindcreek','-d','package_restore','--clean','--if-exists','--exit-on-error','--no-owner'],stdin=source,env=dep.docker_env(),check=True)
 sql='SELECT count(*) FROM knowledge_bases; SELECT count(*) FROM users; SELECT count(*) FROM mindcreek.enterprise_installation'
 original=dep.compose(['exec','-T','postgres','psql','-U','mindcreek','-d','mindcreek_r4','-Atc',sql],True)
 restored=dep.compose(['exec','-T','postgres','psql','-U','mindcreek','-d','package_restore','-Atc',sql],True)
 check('database_backup_restore_native_and_product_schemas',original==restored and original.strip())
 report['status']='passed_with_limitations'
 report['limitations']=[{'code':'amd64_emulation_bm25_abort','status':'unresolved_on_physical_amd64','observed':'Default hybrid retrieval and a minimal BM25 query abort the AMD64 PostgreSQL process under this ARM64 Docker Desktop host. Same-version cached ARM64 query passes. Cause is not proven; JIT/parallel switches did not resolve it.','evidence':'bm25-diagnostic.json','impact':'Hybrid/keyword retrieval is not accepted. Vector-only KB question answering is tested separately; shipped native defaults are unchanged. Run bin/check-database on the target Linux AMD64 host before installing business data.'}]
except Exception as exc:
 report['status']='failed';report['failure']=str(exc)
 try:(STATE/'docker-tail.log').write_text(dep.compose(['logs','--tail','100'],True))
 except Exception:pass
 raise
finally:
 try:dep.compose(['--profile','tls','--profile','tools','down','--volumes','--remove-orphans']);report['disposable_project_removed']=True
 except Exception:report['disposable_project_removed']=False
 report['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat()
 (LOCAL/'smoke.json').write_text(json.dumps(report,indent=2)+'\n')
 print('Report: '+str(LOCAL/'smoke.json'),flush=True)
