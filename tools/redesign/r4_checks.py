#!/usr/bin/env python3
"""Current R4 executable checks; only cached local/synthetic dependencies."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
from r4_evidence import ROOT, source_digest, inputs_digest


def main():
    target=ROOT/'.local/redesign-r4'
    target.mkdir(parents=True,exist_ok=True)
    report={'time':datetime.datetime.now(datetime.timezone.utc).isoformat(), 'status':'running',
            'gateway_source_sha256':source_digest(), 'r4_inputs_sha256':inputs_digest(), 'checks':[],
            'scope':'Synthetic executable regression; R4 native UI checks. Actual synthetic browser/API evidence is recorded separately; no production deployment acceptance.'}
    env={**os.environ,'GOCACHE':str(ROOT/'.local/gateway-go-build'),'GOPROXY':'off','GOSUMDB':'off'}
    node=os.environ.get('R4_NODE_BIN','node')
    version=subprocess.check_output([node,'--version'],text=True).strip()
    if not version.startswith('v24.'): raise RuntimeError('R4 frontend compatibility requires Node 24 via R4_NODE_BIN')
    if '/' in node: env['PATH']=str(Path(node).parent)+os.pathsep+env['PATH']
    report['node']=version
    def run(name,args,override=None):
        log=target/(name+'.log'); start=time.monotonic()
        with log.open('w') as stream:
            result=subprocess.run(args,cwd=ROOT,env={**env,**(override or {})},stdout=stream,stderr=subprocess.STDOUT)
        report['checks'].append({'name':name,'command':args,'exit_code':result.returncode,
            'elapsed_seconds':round(time.monotonic()-start,2),'log_sha256':hashlib.sha256(log.read_bytes()).hexdigest()})
        if result.returncode: raise RuntimeError(name+' failed; see '+str(log))
        print('PASS '+name,flush=True)
    try:
        run('gateway-uncached',['go','-C','services/gateway','test','-count=1','./...'])
        run('gateway-race',['go','-C','services/gateway','test','-race','-count=1','./internal/nativeaccess','./internal/mcp','./internal/enterprise','./internal/identity','./internal/weknora','./internal/server'])
        run('phase0',['make','phase0-check'])
        run('phase5',['make','phase5-check'])
        run('stage1',['make','stage1-check'])
        run('frontend-node24',['sh','scripts/r4-build-ui.sh'])
        image=subprocess.check_output(['docker','image','inspect','python:3.12-alpine','--format','{{.Id}}'],text=True).strip()
        run('importer-python312',['docker','run','--rm','--network','none','-v',str(ROOT/'tools/community-import')+':/work:ro','-w','/work',image,'python','-m','unittest','discover','-s','tests'])
        run('r3-route-manifest',['python3','tools/redesign/r3_routes.py','--inventory','.local/redesign-r3/upstream-routes.json','--check'])
        fixture=target/'compose-fixture'; fixture.mkdir(exist_ok=True)
        secrets=fixture/'secrets'; secrets.mkdir(exist_ok=True)
        envfile=fixture/'synthetic.env'; envfile.write_text((ROOT/'deploy/r4/.env.example').read_text());envfile.chmod(0o600)
        run('r4-compose',['scripts/r4-compose.sh','config','--quiet'],{'MINDCREEK_R4_ENV_FILE':str(envfile),'MINDCREEK_R4_SECRET_DIR':str(secrets)})
        run('r4-shell',['sh','-n','scripts/r4-compose.sh','scripts/r4-install.sh','scripts/r2-install.sh'])
        if report['gateway_source_sha256']!=source_digest() or report['r4_inputs_sha256']!=inputs_digest():
            raise RuntimeError('Source changed during checks; rerun affected verification')
        if subprocess.check_output(['git','-C','upstream/weknora','status','--porcelain'],cwd=ROOT,text=True).strip():
            raise RuntimeError('Upstream source is dirty')
        report['upstream_clean']=True
        report['status']='passed'
    except Exception as error:
        report['status']='failed';report['error']=str(error)
        raise
    finally:
        (ROOT/'docs/plans/evidence/r4-checks.json').write_text(json.dumps(report,indent=2)+'\n')


if __name__=='__main__':main()
