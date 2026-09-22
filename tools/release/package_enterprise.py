#!/usr/bin/env python3
"""Assemble/export the validated R4 AMD64 package, without instance secrets."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile

ROOT=Path(__file__).resolve().parents[2]
LOCAL=ROOT/'.local/enterprise-amd64'


def runtime_digest(target):
    h=hashlib.sha256()
    for p in sorted(target.rglob('*')):
        if p.is_file() and (p.parts[len(target.parts)] in ['bin','config','scripts','deploy'] or p.name in ['compose.json','enterprise.env.example']) and '__pycache__' not in p.parts:
            h.update(str(p.relative_to(target)).encode()+b'\0'+p.read_bytes()+b'\0')
    return h.hexdigest()


def archive_configs(target, record):
    with tarfile.open(target/'images.tar') as tar:
        entries=json.load(tar.extractfile('manifest.json'))
        if len(entries)!=len(record['images']):raise RuntimeError('Wrong image count in archive')
        configs={}
        for entry in entries:
            raw=tar.extractfile(entry['Config']).read();cfg=json.loads(raw)
            if (cfg['os'],cfg['architecture'])!=('linux','amd64'):raise RuntimeError('Mixed image platform')
            for tag in entry['RepoTags']:configs[tag]='sha256:'+hashlib.sha256(raw).hexdigest()
        for image in record['images']:image['config_digest']=configs[image['name']]


def assemble():
    record=json.loads((LOCAL/'build.json').read_text())
    target=ROOT/'images/archives'/('mindcreek-'+record['release']+'-amd64')
    target.mkdir(parents=True,exist_ok=True)
    for directory in ['bin','config','scripts','deploy/phase5','licenses','evidence']:
        (target/directory).mkdir(exist_ok=True,parents=True)
    if not (LOCAL/'public-ca.crt').exists():
        with (LOCAL/'public-ca.crt').open('wb') as out:
            subprocess.run(['docker','run','--rm','--pull','never','--platform','linux/amd64','--entrypoint','cat','nginx:1.30.3-alpine','/etc/ssl/certs/ca-certificates.crt'],stdout=out,check=True)
    mapping={
        'deploy/enterprise/compose.json':'compose.json',
        'deploy/enterprise/enterprise.env.example':'enterprise.env.example',
        'deploy/enterprise/mindcreek.py':'bin/mindcreek',
        'deploy/enterprise/check_database.py':'bin/check-database',
        'deploy/enterprise/tls.conf':'config/tls.conf',
        'deploy/enterprise/nginx.frontend.conf':'config/nginx.frontend.conf',
        'upstream/weknora/frontend/nginx-api-proxy.conf':'config/nginx-api-proxy.conf',
        'upstream/weknora/config/config.yaml':'config/upstream.yaml',
        'deploy/phase5/builtin_agents.yaml':'config/builtin_agents.yaml',
        'deploy/phase5/builtin_models.yaml.tmpl':'deploy/phase5/builtin_models.yaml.tmpl',
        'scripts/render-phase5-models.py':'scripts/render-phase5-models.py',
        'upstream/weknora/LICENSE':'licenses/WeKnora-LICENSE',
        '.local/enterprise-amd64/public-ca.crt':'config/public-ca.crt',
    }
    for name in ['phase1-route-policy.json','phase2-route-actions.json','r3-routes.json','r3-capabilities.json']:
        mapping['config/'+name]='config/'+name
    for src,dest in mapping.items(): shutil.copy2(ROOT/src,target/dest)
    (target/'bin/mindcreek').chmod(0o755)
    guide=ROOT/'docs/guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md'
    if guide.exists(): shutil.copy2(guide,target/'DEPLOYMENT_ZH.md')
    for image in record['images']:
        # Docker save by tag restores an offline-usable name, including Nginx.
        image['name']=image['name'].split('@')[0]
        actual=json.loads(subprocess.check_output(['docker','image','inspect','--platform','linux/amd64',image['name']]))[0]
        assert actual['Id']==image['id'] and actual['Architecture']=='amd64'
    if (target/'images.tar').exists():archive_configs(target,record)
    record['package_runtime_sha256']=runtime_digest(target)
    (target/'RELEASE.json').write_text(json.dumps(record,indent=2)+'\n')
    (target/'licenses/THIRD_PARTY.md').write_text('''# Third-party runtime images

Runtime image names, exact local configuration digests and registry digests are
in RELEASE.json. Original license notices remain inside the unmodified images.
WeKnora source: https://github.com/Tencent/WeKnora (pinned commit in RELEASE.json).
Other runtime projects: https://github.com/paradedb/paradedb,
https://github.com/redis/redis (Redis 7.0), https://nginx.org,
https://www.python.org, and their OS/transitive dependencies.
The package contains no model weights. Review the upstream image notices for your
organization's internal redistribution requirements.
''')
    (LOCAL/'package-path').write_text(str(target)+'\n')
    print(target)
    return target


def export(target):
    destination=target/'images.tar'
    if destination.exists(): raise RuntimeError('images.tar already exists; inspect before replacing')
    images=[v['name'] for v in json.loads((target/'RELEASE.json').read_text())['images']]
    temp=target/'images.tar.partial'
    subprocess.run(['docker','image','save','--platform','linux/amd64','--output',str(temp),*images],check=True)
    temp.rename(destination)
    record=json.loads((target/'RELEASE.json').read_text());archive_configs(target,record)
    (target/'RELEASE.json').write_text(json.dumps(record,indent=2)+'\n')
    print('Exported '+str(destination))


def sha(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda:f.read(8*1024*1024),b''):h.update(block)
    return h.hexdigest()


def finalize(target):
    assert (target/'images.tar').is_file() and (target/'DEPLOYMENT_ZH.md').is_file()
    proof=json.loads((LOCAL/'acceptance.json').read_text())
    if proof['status'] not in ['passed','passed_with_limitations']:raise RuntimeError('Package smoke acceptance has not completed')
    if proof['status']=='passed_with_limitations' and not proof.get('limitations'):raise RuntimeError('Missing explicit limitation evidence')
    if proof['release']!=json.loads((target/'RELEASE.json').read_text())['release']:raise RuntimeError('Wrong smoke release')
    if proof['package_runtime_sha256']!=runtime_digest(target):raise RuntimeError('Runtime package changed after smoke verification')
    record=json.loads((target/'RELEASE.json').read_text())
    record['verification_status']=proof['status']
    record['limitations']=proof.get('limitations',[])
    (target/'RELEASE.json').write_text(json.dumps(record,indent=2)+'\n')
    shutil.copy2(LOCAL/'acceptance.json',target/'evidence/amd64-smoke.json')
    shutil.copy2(LOCAL/'smoke.json',target/'evidence/smoke.json')
    for name in ['restore-check.json','archive-check.json','database-preflight.json','bm25-diagnostic.json','bm25-diagnostic-arm64.json']:
        if (LOCAL/name).exists():shutil.copy2(LOCAL/name,target/'evidence'/name)
    if (target/'instance').exists():raise RuntimeError('Do not bundle installation state')
    files=sorted(p for p in target.rglob('*') if p.is_file() and p.name!='SHA256SUMS' and '__pycache__' not in p.parts)
    (target/'SHA256SUMS').write_text(''.join(sha(p)+'  '+str(p.relative_to(target))+'\n' for p in files))
    archive=target.with_suffix('.tar.gz')
    if archive.exists():raise RuntimeError('Final archive already exists; never silently overwrite')
    with tarfile.open(str(archive)+'.partial','w:gz',compresslevel=1) as tar:
        tar.add(target,arcname=target.name,filter=lambda info: None if '__pycache__' in info.name else info)
    Path(str(archive)+'.partial').rename(archive)
    digest=sha(archive)
    Path(str(archive)+'.sha256').write_text(digest+'  '+archive.name+'\n')
    print(json.dumps({'path':str(archive),'bytes':archive.stat().st_size,'sha256':digest},indent=2))


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('command',choices=['assemble','export','finalize']);a=p.parse_args()
    if a.command=='assemble':assemble()
    else:
        target=Path((LOCAL/'package-path').read_text().strip())
        {'export':export,'finalize':finalize}[a.command](target)
