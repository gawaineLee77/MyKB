#!/usr/bin/env python3
"""Read every final compressed payload and verify the outer and inner checksums."""
import hashlib
import json
import os
from pathlib import Path
import tarfile
from package_enterprise import sha
from release_baselines import UPGRADE_BASELINES
ROOT=Path(__file__).resolve().parents[2];LOCAL=Path(os.environ.get('MINDCREEK_CURRENT_WORKDIR',ROOT/'.local/enterprise-current'))
package=Path((LOCAL/'package-path').read_text().strip());archive=package.with_suffix('.tar.gz')
expected=Path(str(archive)+'.sha256').read_text().split()[0]
assert sha(archive)==expected,'Outer checksum mismatch'
checks={};actual={};names=[];acceptance=None
with tarfile.open(archive,'r|gz') as stream:
    for item in stream:
        parts=Path(item.name).parts
        assert parts and parts[0]==package.name and '..' not in parts
        assert not item.issym() and not item.islnk()
        if not item.isfile():continue
        relative='/'.join(parts[1:]);names.append(relative)
        assert not any(v in parts for v in ('instance','.local','__pycache__','secrets'))
        assert not relative.endswith('enterprise.env')
        content=stream.extractfile(item)
        if relative=='SHA256SUMS':
            checks={name:digest for digest,name in (line.split('  ',1) for line in content.read().decode().splitlines())}
        elif relative=='evidence/amd64-smoke.json':
            raw=content.read();acceptance=json.loads(raw)
            actual[relative]=hashlib.sha256(raw).hexdigest()
        else:
            h=hashlib.sha256()
            for block in iter(lambda:content.read(8*1024*1024),b''):h.update(block)
            actual[relative]=h.hexdigest()
assert checks==actual,'Inner archive checksum mismatch'
assert {'images.tar','RELEASE.json','bin/mindcreek','bin/release','bin/check-database','bin/image-resources','DEPLOYMENT_ZH.md',
        'docs/guides/UPGRADE_R4_AND_20261009_ZH.md',
        'docs/guides/IMAGE_PERMISSION_RECOVERY_ZH.md',
        'evidence/amd64-smoke.json','evidence/validation.json'} <= set(names)
assert {'evidence/'+directory+'/'+filename for directory in UPGRADE_BASELINES
        for filename in ('smoke.json','upgrade-marker.json','cold-rollback.json')} <= set(names)
assert acceptance is not None,'Missing acceptance report'
for limitation in acceptance.get('limitations',[]):
    if limitation.get('evidence'):
        assert 'evidence/'+limitation['evidence'] in names,'Missing limitation evidence: '+limitation['evidence']
report={'status':'passed','release':json.loads((package/'RELEASE.json').read_text())['release'],
        'archive':str(archive),'bytes':archive.stat().st_size,'sha256':expected,'verified_files':len(actual),
        'outer_checksum':True,'every_inner_file_checksum':True,'limitation_evidence_present':True,'instance_secrets_excluded':True}
(LOCAL/'final-archive-check.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
