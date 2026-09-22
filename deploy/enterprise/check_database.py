#!/usr/bin/env python3
"""Run a disposable AMD64 BM25 preflight before installing company data."""
import argparse
import datetime
import json
from pathlib import Path
import platform
import secrets
import subprocess
import tempfile
import time

ROOT=Path(__file__).resolve().parents[1]


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True,help='Report path outside the immutable package')
    args=parser.parse_args()
    record=json.loads((ROOT/'RELEASE.json').read_text())
    image=next(v for v in record['images'] if v['name'].startswith('paradedb/'))
    inspected=json.loads(subprocess.check_output(['docker','image','inspect','--platform','linux/amd64',image['name']]))[0]
    if inspected['Id'] not in [image['id'],image.get('config_digest')] or inspected['Architecture']!='amd64':
        raise SystemExit('Database image does not match RELEASE.json; load the package first.')
    name='mindcreek-database-check-'+secrets.token_hex(6)
    report={'release':record['release'],'time':datetime.datetime.now(datetime.timezone.utc).isoformat(),'host_os':platform.system(),'host_arch':platform.machine(),'image':image,'status':'failed','scope':'Isolated synthetic PostgreSQL/BM25 query; no business volumes, ports, or network access.'}
    def docker(*a,**kwargs):return subprocess.run(['docker',*a],text=True,capture_output=True,check=True,**kwargs).stdout
    with tempfile.TemporaryDirectory(prefix='mindcreek-database-check-') as temporary:
        secret=Path(temporary)/'password';secret.write_text(secrets.token_hex(24));secret.chmod(0o600)
        try:
            docker('run','-d','--pull','never','--platform','linux/amd64','--network','none','--name',name,'-e','POSTGRES_PASSWORD_FILE=/run/password','-v',str(secret)+':/run/password:ro','--tmpfs','/var/lib/postgresql/data',image['name'])
            deadline=time.monotonic()+180
            while True:
                try:docker('exec',name,'pg_isready','-h','127.0.0.1','-U','postgres');break
                except subprocess.CalledProcessError:
                    if time.monotonic()>deadline:raise RuntimeError('Database startup timeout')
                    time.sleep(1)
            sql="""CREATE EXTENSION IF NOT EXISTS pg_search;
CREATE TABLE docs(id SERIAL PRIMARY KEY,content text,knowledge_base_id text,is_enabled boolean);
INSERT INTO docs(content,knowledge_base_id,is_enabled) VALUES ('The synthetic recovery code is AMD64-VERIFIED','kb',true);
CREATE INDEX docs_search ON docs USING bm25(id,content) WITH (key_field='id');
SELECT paradedb.score(id),content FROM docs WHERE knowledge_base_id='kb' AND content ||| 'synthetic recovery' AND (is_enabled IS NULL OR is_enabled=true) ORDER BY paradedb.score(id) DESC LIMIT 10;
"""
            output=docker('exec','-i',name,'psql','-U','postgres','-v','ON_ERROR_STOP=1',input=sql)
            if 'AMD64-VERIFIED' not in output:raise RuntimeError('BM25 returned no synthetic result')
            report['status']='passed'
        except (subprocess.CalledProcessError,RuntimeError) as exc:
            report['error']=exc.stderr[-1500:] if isinstance(exc,subprocess.CalledProcessError) else str(exc)
            logs=subprocess.run(['docker','logs','--tail','12',name],text=True,capture_output=True)
            report['diagnostics']=(logs.stdout+logs.stderr)[-4000:]
        finally:
            cleanup=subprocess.run(['docker','rm','-f','-v',name],text=True,capture_output=True)
            report['disposable_container_removed']=cleanup.returncode==0
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(report,indent=2)+'\n')
    print('Database preflight: '+report['status']+'; report: '+str(args.output))
    if report['status']!='passed':
        print('Stop before importing company data; investigate this report on the target AMD64 host.')
        return 1
    return 0


if __name__=='__main__':raise SystemExit(main())
