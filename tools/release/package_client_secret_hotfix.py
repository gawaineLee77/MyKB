#!/usr/bin/env python3
"""Assemble/finalize the separately verified Client Secret AMD64 gateway patch."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile

ROOT = Path(__file__).resolve().parents[2]
LOCAL = ROOT / '.local/enterprise-client-secret'
BASE = ROOT / 'images/archives/mindcreek-r4-20260916-118b8af3-amd64'
RELEASE = 'r4-20260916-short-secret-3959a2e2'
TARGET = ROOT / 'images/archives' / ('mindcreek-' + RELEASE + '-amd64-hotfix')
sys.path.insert(0, str(ROOT / 'tools/redesign'))
from r4_evidence import source_digest
from apply_gateway_hotfix import sha


def checksums():
    (TARGET / 'SHA256SUMS').write_text(''.join(
        sha(path) + '  ' + str(path.relative_to(TARGET)) + '\n'
        for path in sorted(TARGET.rglob('*'))
        if path.is_file() and path.name != 'SHA256SUMS' and '__pycache__' not in path.parts
    ))


def assemble():
    assert source_digest().startswith('3959a2e2'), 'Source changed after this hotfix was built'
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
    record = {'release': RELEASE, 'base_release': json.loads((BASE / 'RELEASE.json').read_text())['release'],
              'base_release_sha256': sha(BASE / 'RELEASE.json'),
              'base_gateway_image': 'mindcreek-gateway:r4-20260916-118b8af3',
              'gateway_source_sha256': source_digest(), 'gateway_binary_sha256': sha(LOCAL / 'gateway'),
              'image': {'name': image, 'id': info['Id'], 'config_digest': 'sha256:' + hashlib.sha256(raw).hexdigest(),
                        'os': 'linux', 'architecture': 'amd64'},
              'scope': 'Remove the minimum length for nonempty provider-issued OAuth2/OIDC Client Secrets. No database migration.'}
    (TARGET / 'PATCH.json').write_text(json.dumps(record, indent=2) + '\n')
    shutil.copy2(ROOT / 'tools/release/apply_gateway_hotfix.py', TARGET / 'apply.py')
    shutil.copy2(ROOT / 'docs/guides/R4_OAUTH_CLIENT_SECRET_HOTFIX_ZH.md', TARGET / 'README_ZH.md')
    checksums()
    print(TARGET)


def finalize():
    proof = json.loads((LOCAL / 'evidence.json').read_text())
    patch = json.loads((TARGET / 'PATCH.json').read_text())
    assert proof['status'] == 'passed' and proof['gateway_source_sha256'] == source_digest() == patch['gateway_source_sha256']
    assert proof['image_id'] == patch['image']['id']
    assert proof['apply_script_sha256'] == sha(TARGET / 'apply.py')
    shutil.copy2(LOCAL / 'evidence.json', TARGET / 'evidence.json')
    checksums()
    archive = Path(str(TARGET) + '.tar.gz')
    if archive.exists():
        raise ValueError('Final archive already exists; refusing to overwrite')
    temporary = Path(str(archive) + '.partial')
    with tarfile.open(temporary, 'w:gz') as tar:
        tar.add(TARGET, arcname=TARGET.name)
    temporary.rename(archive)
    digest = sha(archive)
    Path(str(archive) + '.sha256').write_text(digest + '  ' + archive.name + '\n')
    print(json.dumps({'path': str(archive), 'bytes': archive.stat().st_size, 'sha256': digest}, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['assemble', 'finalize'])
    args = parser.parse_args()
    {'assemble': assemble, 'finalize': finalize}[args.command]()
