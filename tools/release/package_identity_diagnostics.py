#!/usr/bin/env python3
"""Package the separately verified AMD64 identity diagnostics gateway."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile

ROOT = Path(__file__).resolve().parents[2]
LOCAL = ROOT / '.local/identity-diagnostics'
BASE = ROOT / 'images/archives/mindcreek-r4-20260916-118b8af3-amd64'
RELEASE = 'r4-20260916-identity-diag1'
TARGET = ROOT / 'images/archives' / ('mindcreek-' + RELEASE + '-amd64-hotfix')
sys.path.insert(0, str(ROOT / 'tools/redesign'))
from r4_evidence import source_digest
from apply_gateway_hotfix import sha


def checksums():
    (TARGET / 'SHA256SUMS').write_text(''.join(sha(p) + '  ' + str(p.relative_to(TARGET)) + '\n'
        for p in sorted(TARGET.rglob('*')) if p.is_file() and p.name != 'SHA256SUMS' and '__pycache__' not in p.parts))


def assemble():
    build = json.loads((LOCAL / 'build.json').read_text())
    assert build['gateway_source_sha256'] == source_digest()
    assert build['gateway_binary_sha256'] == sha(LOCAL / 'gateway')
    TARGET.mkdir(parents=True, exist_ok=True)
    image = 'mindcreek-gateway:' + RELEASE
    info = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', image]))[0]
    assert (info['Os'], info['Architecture']) == ('linux', 'amd64')
    archive = TARGET / 'gateway-image.tar'
    if not archive.exists():
        temporary = archive.with_suffix('.tar.partial')
        subprocess.run(['docker', 'image', 'save', '--platform', 'linux/amd64', '--output', str(temporary), image], check=True)
        temporary.rename(archive)
    with tarfile.open(archive) as tar:
        manifest = json.load(tar.extractfile('manifest.json'))
        assert len(manifest) == 1 and manifest[0]['RepoTags'] == [image]
        raw = tar.extractfile(manifest[0]['Config']).read()
        config = json.loads(raw)
        assert (config['os'], config['architecture']) == ('linux', 'amd64')
        assert config['config']['Labels']['org.opencontainers.image.version'] == RELEASE
        binary_matches = False
        for layer in manifest[0]['Layers']:
            with tarfile.open(fileobj=tar.extractfile(layer), mode='r:*') as content:
                for entry in content:
                    if entry.name.lstrip('./') == 'mindcreek-gateway' and entry.isfile():
                        binary_matches = hashlib.sha256(content.extractfile(entry).read()).hexdigest() == build['gateway_binary_sha256']
        assert binary_matches, 'Exported image does not contain the verified binary'
    patch = {**build, 'release': RELEASE, 'base_release': json.loads((BASE / 'RELEASE.json').read_text())['release'],
             'base_release_sha256': sha(BASE / 'RELEASE.json'), 'base_gateway_image': 'mindcreek-gateway:r4-20260916-118b8af3',
             'replaces': ['mindcreek-gateway:r4-20260916-short-secret-3959a2e2'],
             'image': {'name': image, 'id': info['Id'], 'config_digest': 'sha256:' + hashlib.sha256(raw).hexdigest(), 'os': 'linux', 'architecture': 'amd64'},
             'scope': 'Redacted authentication diagnostics. Retains short Client Secret fix. No protocol setting, UI, upstream or database migration changes.'}
    (TARGET / 'PATCH.json').write_text(json.dumps(patch, indent=2) + '\n')
    for source, destination in [('tools/release/apply_gateway_hotfix.py', 'apply.py'),
                                ('tools/release/collect_identity_logs.py', 'collect.py'),
                                ('docs/guides/R4_IDENTITY_DIAGNOSTICS_ZH.md', 'README_ZH.md')]:
        shutil.copy2(ROOT / source, TARGET / destination)
    checksums()
    print(TARGET)


def finalize():
    proof = json.loads((LOCAL / 'evidence.json').read_text())
    patch = json.loads((TARGET / 'PATCH.json').read_text())
    assert proof['status'] == 'passed' and proof['gateway_source_sha256'] == source_digest() == patch['gateway_source_sha256']
    assert proof['image_id'] == patch['image']['id']
    assert proof['apply_script_sha256'] == sha(TARGET / 'apply.py')
    assert proof['collect_script_sha256'] == sha(TARGET / 'collect.py')
    shutil.copy2(LOCAL / 'evidence.json', TARGET / 'evidence.json')
    checksums()
    archive = Path(str(TARGET) + '.tar.gz')
    if archive.exists():
        raise ValueError('Final archive already exists; refusing to overwrite')
    temporary = Path(str(archive) + '.partial')
    with tarfile.open(temporary, 'w:gz') as tar:
        tar.add(TARGET, arcname=TARGET.name, filter=lambda entry: None if '__pycache__' in entry.name else entry)
    temporary.rename(archive)
    digest = sha(archive)
    Path(str(archive) + '.sha256').write_text(digest + '  ' + archive.name + '\n')
    print(json.dumps({'path': str(archive), 'bytes': archive.stat().st_size, 'sha256': digest}, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['assemble', 'finalize'])
    args = parser.parse_args()
    {'assemble': assemble, 'finalize': finalize}[args.command]()
