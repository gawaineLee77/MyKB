#!/usr/bin/env python3
"""Disposable AMD64 pg_search diagnostic; no company data or existing volumes."""
import argparse,json,secrets,subprocess,tempfile,time
from pathlib import Path
root=Path(__file__).resolve().parents[2];work=Path(tempfile.mkdtemp(prefix='bm25-',dir=root/'.local/enterprise-amd64'))
name='mindcreek-bm25-'+secrets.token_hex(4);password=work/'password';password.write_text(secrets.token_hex(16));password.chmod(0o600)
p=argparse.ArgumentParser();p.add_argument('--platform',default='linux/amd64');p.add_argument('--image',default='paradedb/paradedb:v0.22.2-pg17');args=p.parse_args()
def docker(*args,**kwargs):return subprocess.run(['docker',*args],check=True,capture_output=True,text=True,**kwargs).stdout
report={'container':name,'platform':args.platform,'checks':[]}
try:
 docker('run','-d','--pull','never','--platform',args.platform,'--name',name,'-e','POSTGRES_PASSWORD_FILE=/run/password','-v',str(password)+':/run/password:ro','--tmpfs','/var/lib/postgresql/data',args.image)
 for _ in range(100):
  try:docker('exec',name,'pg_isready','-h','127.0.0.1','-U','postgres');break
  except subprocess.CalledProcessError:time.sleep(1)
 setup="""CREATE EXTENSION IF NOT EXISTS pg_search;
CREATE TABLE docs(id SERIAL PRIMARY KEY,content text,knowledge_base_id text,is_enabled boolean);
INSERT INTO docs(content,knowledge_base_id,is_enabled) VALUES ('The synthetic recovery code is AMD64-VERIFIED','kb',true),('PDF verification recovery code PDF-VERIFIED','kb',true);
CREATE INDEX docs_search ON docs USING bm25(id,content) WITH (key_field='id');
"""
 print(docker('exec','-i',name,'psql','-U','postgres','-v','ON_ERROR_STOP=1',input=setup),flush=True)
 for mode,prefix in [('default',''),('jit_off','SET jit=off;'),('custom_off','SET pg_search.enable_custom_scan=off;'),('parallel_off','SET max_parallel_workers_per_gather=0;'),('both_off','SET jit=off; SET max_parallel_workers_per_gather=0;')]:
  sql=prefix+"SELECT paradedb.score(id),content FROM docs WHERE knowledge_base_id='kb' AND content ||| 'What is the synthetic recovery code?' AND (is_enabled IS NULL OR is_enabled=true) ORDER BY paradedb.score(id) DESC LIMIT 10;"
  try:out=docker('exec','-i',name,'psql','-U','postgres','-v','ON_ERROR_STOP=1',input=sql);result={'mode':mode,'passed':True,'output':out}
  except subprocess.CalledProcessError as e:
   result={'mode':mode,'passed':False,'error':e.stderr[-1000:]};time.sleep(4)
  report['checks'].append(result);print(json.dumps(result),flush=True)
 report['logs']=docker('logs','--tail','20',name)
finally:
 docker('rm','-f',name)
 (root/('.local/enterprise-amd64/bm25-diagnostic'+('' if args.platform=='linux/amd64' else '-'+args.platform.split('/')[-1])+'.json')).write_text(json.dumps(report,indent=2)+'\n')
