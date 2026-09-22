#!/usr/bin/env python3
"""Build AMD64 runtime images from verified R4 or R5 sources/artifacts."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools/redesign'))
from r4_evidence import source_digest, ui_digest

NGINX = 'nginx:1.30.3-alpine@sha256:0d3b80406a13a767339fbe2f41406d6c7da727ab89cf8fae399e81f780f814d1'
EXTERNAL = ['wechatopenai/weknora-app:v0.8.0', 'wechatopenai/weknora-docreader:v0.8.0',
            'paradedb/paradedb:v0.22.2-pg17', 'redis:7.0-alpine', 'python:3.12-alpine', NGINX]
LOCAL = ROOT / '.local/enterprise-amd64'

def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)

def inspect(name):
    record = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', name], text=True))[0]
    if (record['Os'], record['Architecture']) != ('linux', 'amd64'):
        raise RuntimeError('Incorrect platform: ' + name)
    return {'name': name, 'id': record['Id'], 'os': record['Os'], 'architecture': record['Architecture'],
            'repo_digests': record.get('RepoDigests', [])}

def build(profile='r4'):
    if profile == 'r5':
        from r5_evidence import ui_digest as current_ui_digest
    else:
        current_ui_digest = ui_digest
    LOCAL.mkdir(parents=True, exist_ok=True)
    proof = json.loads((ROOT / f'docs/plans/evidence/{profile}-ui-build.json').read_text())
    if proof['status'] != 'passed' or proof['ui_source_sha256'] != current_ui_digest():
        raise RuntimeError(f'Run Node 24 make {profile}-ui-build for the current sources first')
    ui = Path((ROOT / f'.local/redesign-{profile}/ui-path').read_text().strip())
    digest = hashlib.sha256()
    for path in sorted((ui / 'dist').rglob('*')):
        if path.is_file(): digest.update(str(path.relative_to(ui / 'dist')).encode() + b'\0' + path.read_bytes())
    if digest.hexdigest() != proof['bundle_sha256']: raise RuntimeError('Frontend artifact changed')
    if subprocess.check_output(['git','-C','upstream/weknora','status','--porcelain'], cwd=ROOT, text=True).strip():
        raise RuntimeError('Pinned upstream must remain clean')
    release = profile + '-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d') + '-' + source_digest()[:8]
    for name in EXTERNAL: inspect(name)  # Fail before build if an AMD64 dependency is missing.
    # A local tag avoids registry metadata resolution during the offline build.
    run(['docker', 'tag', NGINX, 'mindcreek-release-base:nginx-amd64'])
    run(['docker', 'tag', NGINX, 'nginx:1.30.3-alpine'])
    context = LOCAL / 'ui'; context.mkdir(exist_ok=True)
    shutil.copytree(ui / 'dist', context / 'dist', dirs_exist_ok=True)
    # The release uses the product Nginx template and the selected UI artifact.
    shutil.copy2(ROOT / 'deploy/enterprise/nginx.frontend.conf', context / 'nginx.conf')
    shutil.copy2(ROOT / 'upstream/weknora/frontend/nginx-api-proxy.conf', context)
    shutil.copy2(ROOT / 'upstream/weknora/frontend/docker-entrypoint.sh', context)
    shutil.copy2(ROOT / 'upstream/weknora/LICENSE', context)
    for name, directory in [('ui', context), ('gateway', LOCAL / 'gateway')]:
        directory.mkdir(exist_ok=True)
        if name == 'gateway':
            env = {**os.environ, 'CGO_ENABLED':'0', 'GOOS':'linux', 'GOARCH':'amd64',
                   'GOCACHE':str(ROOT / '.local/gateway-go-build'), 'GOPROXY':'off', 'GOSUMDB':'off'}
            run(['go','-C','services/gateway','build','-trimpath','-ldflags',
                 '-s -w -X main.productVersion=' + release,'-o',str(directory / 'gateway'),'./cmd/gateway'], env=env)
        run(['docker','buildx','build','--platform','linux/amd64','--load','--pull=false','--network','none',
             '--provenance=false','--tag','mindcreek-' + name + ':' + release,'--build-arg','RELEASE_ID=' + release,
             '--build-arg','NGINX_BASE=mindcreek-release-base:nginx-amd64',
             '--file',str(ROOT / ('images/mindcreek-' + name + '/Dockerfile.release')),str(directory)])
    images = [inspect(name) for name in ['mindcreek-ui:' + release, 'mindcreek-gateway:' + release, *EXTERNAL]]
    record = {'release':release, 'platform':'linux/amd64', 'created_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'gateway_source_sha256':source_digest(), 'ui_source_sha256':current_ui_digest(), 'ui_bundle_sha256':proof['bundle_sha256'],
              'frontend_nginx_sha256':hashlib.sha256((context/'nginx.conf').read_bytes()).hexdigest(),
              'frontend_proxy_sha256':hashlib.sha256((context/'nginx-api-proxy.conf').read_bytes()).hexdigest(),
              'upstream_commit':subprocess.check_output(['git','-C','upstream/weknora','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),
              'git_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),
              'working_tree_snapshot':True, 'images':images,
              'scope':profile.upper()+' enterprise deployment/debugging bundle; target company OAuth/models and x86 hardware acceptance are separate.'}
    (LOCAL / 'build.json').write_text(json.dumps(record, indent=2) + '\n')
    print('Built ' + release + ': ' + str(len(images)) + ' AMD64 runtime images', flush=True)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile', choices=('r4','r5'),default='r4')
    args = parser.parse_args()
    build(args.profile)
