#!/usr/bin/env python3
"""Reload and inspect every platform configuration in the generated image tar."""
import datetime,hashlib,json,subprocess,tarfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];LOCAL=ROOT/'.local/enterprise-amd64';target=Path((LOCAL/'package-path').read_text().strip());lock=json.loads((target/'RELEASE.json').read_text())
subprocess.run(['docker','load','--input',str(target/'images.tar')],check=True)
subprocess.run(['python3',str(target/'bin/mindcreek'),'images'],check=True)
rows=[]
with tarfile.open(target/'images.tar') as tar:
 for entry in json.load(tar.extractfile('manifest.json')):
  raw=tar.extractfile(entry['Config']).read();cfg=json.loads(raw);name=entry['RepoTags'][0];expected=next(i for i in lock['images'] if i['name']==name)
  assert (cfg['os'],cfg['architecture'])==('linux','amd64')
  assert 'sha256:'+hashlib.sha256(raw).hexdigest()==expected['config_digest']
  rows.append({'name':name,'platform':'linux/amd64','config_digest':expected['config_digest']})
assert len(rows)==8
for filename,key in [('nginx.frontend.conf','frontend_nginx_sha256'),('nginx-api-proxy.conf','frontend_proxy_sha256')]:
 assert hashlib.sha256((target/'config'/filename).read_bytes()).hexdigest()==lock[key]
record={'status':'passed','release':lock['release'],'images':rows,'archive_bytes':(target/'images.tar').stat().st_size,'docker_load':'passed','frontend_nginx_sources':'match_built_image','time':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(LOCAL/'archive-check.json').write_text(json.dumps(record,indent=2)+'\n')
print('Eight AMD64 image configurations and archive reload verified.')
