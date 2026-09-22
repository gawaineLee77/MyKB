#!/usr/bin/env python3
"""Recheck the corrected pg_restore command against the latest synthetic backup."""
import datetime,importlib.machinery,importlib.util,json,os,secrets,subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];LOCAL=ROOT/'.local/enterprise-amd64';package=Path((LOCAL/'package-path').read_text().strip());state=Path((LOCAL/'smoke-state').read_text().strip())
project='mindcreek-restore-'+secrets.token_hex(4)
os.environ.update(MINDCREEK_STATE_DIR=str(state),MINDCREEK_PROJECT=project)
loader=importlib.machinery.SourceFileLoader('deployment',str(package/'bin/mindcreek'));spec=importlib.util.spec_from_loader(loader.name,loader);dep=importlib.util.module_from_spec(spec);loader.exec_module(dep)
report={'time':datetime.datetime.now(datetime.timezone.utc).isoformat(),'release':dep.release()['release'],'package_runtime_sha256':dep.release()['package_runtime_sha256'],'project':project,'status':'failed','scope':'Fresh disposable database and volumes restored from the current synthetic AMD64 smoke backup, using documented commands.'}
cmd=['docker','compose','-p',project,'--project-directory',str(package),'--env-file','/dev/null','-f',str(package/'compose.json'),'-f',str(state/'compose.override.json')]
try:
 dep.up_services(['postgres'])
 with (state/'backup/database.dump').open('rb') as source:
  subprocess.run([*cmd,'exec','-T','postgres','pg_restore','-U','mindcreek','-d','mindcreek_r4','--clean','--if-exists','--exit-on-error','--no-owner'],stdin=source,env=dep.docker_env(),check=True)
 sql="SELECT count(*) FROM knowledge_bases; SELECT count(*) FROM users; SELECT stage FROM mindcreek.enterprise_installation; SELECT count(*) FROM knowledges;"
 rows=dep.compose(['exec','-T','postgres','psql','-U','mindcreek','-d','mindcreek_r4','-Atc',sql],True).strip().splitlines()
 assert rows==['2','2','ready','3'],rows
 report['restored_rows']={'knowledge_bases':2,'users':2,'installation_stage':'ready','knowledges':3}
 code='''import os,pathlib,tarfile,shutil,hashlib,json
count=0
with tarfile.open('/backup/files-and-redis.tar.gz') as t:
 for m in t:
  parts=pathlib.PurePosixPath(m.name).parts
  if not parts or parts[0] not in ('files','redis') or '..' in parts or m.issym() or m.islnk():raise ValueError('Unsafe entry')
  root=pathlib.Path('/data/files' if parts[0]=='files' else '/redis-data');p=root.joinpath(*parts[1:])
  if m.isdir():p.mkdir(parents=True,exist_ok=True)
  elif m.isfile():
   p.parent.mkdir(parents=True,exist_ok=True)
   data=t.extractfile(m).read();p.write_bytes(data)
   assert hashlib.sha256(p.read_bytes()).digest()==hashlib.sha256(data).digest();count+=1
  else:raise ValueError('Unsupported entry')
  os.chmod(p,m.mode&0o777);os.chown(p,m.uid,m.gid)
assert count>=3
print(json.dumps({'restored_files':count}))
'''
 report.update(json.loads(dep.compose(['run','--rm','--no-deps','--pull','never','ops','-c',code],True)))
 dep.up_services(['redis']);report['redis_after_restore']='healthy'
 report['status']='passed'
finally:
 dep.compose(['--profile','tls','--profile','tools','down','--volumes','--remove-orphans']);report['disposable_project_removed']=True
 (LOCAL/'restore-check.json').write_text(json.dumps(report,indent=2)+'\n')
 print(json.dumps(report,indent=2))
