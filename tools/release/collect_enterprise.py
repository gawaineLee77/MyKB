#!/usr/bin/env python3
"""Combine runtime and corrected recovery evidence without hiding failed checks."""
import datetime,hashlib,json,subprocess
from pathlib import Path
from package_enterprise import runtime_digest
ROOT=Path(__file__).resolve().parents[2];LOCAL=ROOT/'.local/enterprise-amd64';target=Path((LOCAL/'package-path').read_text().strip());release=json.loads((target/'RELEASE.json').read_text())
read=lambda n:json.loads((LOCAL/n).read_text())
runtime,restore,archive,preflight,arm=[read(n) for n in ['smoke.json','restore-check.json','archive-check.json','database-preflight.json','bm25-diagnostic-arm64.json']]
assert runtime['release']==restore['release']==archive['release']==release['release']
assert runtime['package_runtime_sha256']==restore['package_runtime_sha256']==runtime_digest(target)
assert all(c['passed'] for c in runtime['checks']) and len(runtime['checks'])>=19
recovered_failure=runtime['status']=='failed'
if recovered_failure:
 assert runtime['checks'][-1]['name']=='ops_quiesced_files_and_redis_backup'
 assert 'pg_restore' in runtime.get('failure',''), 'Only the documented restore-schema conflict is superseded'
else:assert runtime['status'] in ['passed','passed_with_limitations']
assert restore['status']=='passed' and restore['restored_rows']=={'knowledge_bases':2,'users':2,'installation_stage':'ready','knowledges':3}
assert restore['restored_files']>=3 and restore['redis_after_restore']=='healthy'
assert archive['status']=='passed' and len(archive['images'])==8
assert preflight['status']=='failed' and all(c['passed'] for c in arm['checks'])
assert runtime['disposable_project_removed'] and restore['disposable_project_removed'] and preflight['disposable_container_removed']
unit=subprocess.run(['docker','run','--rm','--pull','never','--platform','linux/amd64','--network','none','-v',str(ROOT)+':/src:ro','-w','/src','python:3.12-alpine','python','-B','-m','unittest','discover','-s','tools/release','-p','test_*.py'],capture_output=True,text=True,check=True)
assert 'Ran 10 tests' in unit.stderr
subprocess.run(['python3','scripts/check-design-docs.py','--r4'],cwd=ROOT,check=True)
subprocess.run(['git','diff','--check'],cwd=ROOT,check=True)
assert not subprocess.check_output(['git','-C',str(ROOT/'upstream/weknora'),'status','--porcelain'],text=True).strip()
report={k:runtime[k] for k in ['release','platform','package_runtime_sha256','host_execution','images']}
report.update(status='passed_with_limitations',time=datetime.datetime.now(datetime.timezone.utc).isoformat(),scope='Combined evidence for the same packaged runtime. Nineteen runtime checks passed; the initial schema-conflict restore failed and was corrected and independently rechecked using its actual backup. Hybrid/BM25 remains unaccepted.',checks=runtime['checks']+[
 {'name':'database_backup_restore_native_and_product_schemas','passed':True,'evidence':'restore-check.json'},
 {'name':'files_and_redis_restore_into_fresh_volumes','passed':True,'evidence':'restore-check.json'}],unit_tests={'runtime':'Python 3.12 linux/amd64','count':10,'passed':True},archive_checks='archive-check.json',source_and_document_checks='current R4 evidence, new guide links/format, diff whitespace and clean pinned upstream',resolved_failure={'step':'initial backup restore','cause':'ParadeDB bootstrap already creates the paradedb schema','correction':'Use pg_restore --clean --if-exists only in a new isolated restore target; native/product data plus files and Redis were rechecked.','evidence':'restore-check.json'},limitations=[{'code':'amd64_emulation_bm25_abort','status':'unresolved_on_physical_amd64','observed':'Default hybrid retrieval and minimal BM25 queries abort PostgreSQL under this ARM host AMD64 emulator. Same-version ARM64 controls pass; the cause is not proven.','impact':'Hybrid and keyword retrieval are not accepted. Vector-only KB question answering passed separately; shipped application defaults remain unchanged. Run bin/check-database on the target Linux AMD64 server before importing business data.','evidence':['database-preflight.json','bm25-diagnostic.json','bm25-diagnostic-arm64.json']}],disposable_projects_removed=True)
report['evidence_sha256']={n:hashlib.sha256((LOCAL/n).read_bytes()).hexdigest() for n in ['smoke.json','restore-check.json','archive-check.json','database-preflight.json','bm25-diagnostic.json','bm25-diagnostic-arm64.json']}
report['checks']=list({c['name']:c for c in report['checks']}.values())
if not recovered_failure:
 report['resolved_failure']=None
 report['scope']='Combined runtime and independent backup recovery evidence for the same package; BM25 remains unaccepted on this host.'
(LOCAL/'acceptance.json').write_text(json.dumps(report,indent=2)+'\n')
print('Collected 21 passing runtime/recovery checks, 10 installer tests, 8 AMD64 image checks; BM25 limitation remains explicit.')
