#!/usr/bin/env python3
"""Package the verified frontend as an offline add-on to the original R4 bundle."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
from configure import sha

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'images/archives/mindcreek-r4-20260916-118b8af3-amd64'
TARGET = ROOT / 'images/archives/mindcreek-r4-20260918-links1-amd64-addon'
IMAGE = 'mindcreek-ui:r4-20260918-links1'


def copy(source, destination=None):
    target = TARGET / (destination or source)
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(ROOT / source, target)


def checksums():
    (TARGET / 'SHA256SUMS').write_text(''.join(
        sha(path) + '  ' + str(path.relative_to(TARGET)) + '\n'
        for path in sorted(TARGET.rglob('*'))
        if path.is_file() and path.name != 'SHA256SUMS' and '__pycache__' not in path.parts))


def assemble():
    TARGET.mkdir(parents=True, exist_ok=True)
    archive = TARGET / 'images.tar'
    subprocess.run(['docker', 'image', 'save', '--platform', 'linux/amd64', '-o', str(archive), IMAGE], check=True)
    with tarfile.open(archive) as stream:
        manifest = json.load(stream.extractfile('manifest.json'))
        assert len(manifest) == 1 and manifest[0]['RepoTags'] == [IMAGE]
        raw = stream.extractfile(manifest[0]['Config']).read()
        config = json.loads(raw)
        assert (config['os'], config['architecture']) == ('linux', 'amd64')
    # Record the ID after import, matching what configure.py sees on the target host.
    subprocess.run(['docker', 'load', '-i', str(archive)], check=True)
    info = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', IMAGE]))[0]
    lock = {'release': TARGET.name, 'base_release_sha256': sha(BASE / 'RELEASE.json'),
        'image': {'name': IMAGE, 'id': info['Id'], 'config_digest': 'sha256:' + hashlib.sha256(raw).hexdigest(),
                  'os': info['Os'], 'architecture': info['Architecture']},
        'scope': 'Frontend navigation links only; no backend, identity, graph or database changes.'}
    (TARGET / 'PATCH.json').write_text(json.dumps(lock, indent=2) + '\n')
    copy('tools/ui-links/configure.py', 'configure.py')
    copy('tools/frontend-overlay/public/mindcreek-links.json', 'links.example.json')
    copy('docs/guides/R4_ENTERPRISE_LINKS_ZH.md')
    (TARGET / 'README_ZH.md').write_text('# MindCreek 企业文档链接补丁包\n\n'
        '请按[部署与配置指南](docs/guides/R4_ENTERPRISE_LINKS_ZH.md)操作，沿用原完整包和实例目录。\n\n'
        'configure.py 只加载前端镜像并合并配置，不启动或停止服务。默认隐藏上游文档链接。\n')
    checksums()
    print(TARGET)


def finalize():
    proof = json.loads((ROOT / 'docs/plans/evidence/ui-links-release.json').read_text())
    assert proof['status'] == 'passed'
    assert proof['helper_sha256'] == sha(ROOT / 'tools/ui-links/configure.py') == sha(TARGET / 'configure.py')
    assert proof['image'] == json.loads((TARGET / 'PATCH.json').read_text())['image']
    assert sha(ROOT / 'docs/guides/R4_ENTERPRISE_LINKS_ZH.md') == sha(TARGET / 'docs/guides/R4_ENTERPRISE_LINKS_ZH.md')
    for path, digest in proof['source_sha256'].items():
        assert sha(ROOT / path) == digest, path
    files = ['docs/plans/R4_UI_LINKS_ACCEPTANCE.md',
        *[str(path.relative_to(ROOT)) for path in sorted((ROOT / 'docs/plans/evidence').glob('ui-links-*.json'))],
        *json.loads((ROOT / 'docs/plans/evidence/ui-links-browser.json').read_text())['screenshots']]
    for path in files:
        copy(path)
    checksums()
    archive = Path(str(TARGET) + '.tar.gz')
    if archive.exists():
        raise ValueError('Archive exists; refusing overwrite')
    partial = Path(str(archive) + '.partial')
    with tarfile.open(partial, 'w:gz', compresslevel=1) as stream:
        stream.add(TARGET, arcname=TARGET.name, filter=lambda entry: None if '__pycache__' in entry.name else entry)
    partial.rename(archive)
    digest = sha(archive)
    Path(str(archive) + '.sha256').write_text(digest + '  ' + archive.name + '\n')
    print(json.dumps({'path': str(archive), 'bytes': archive.stat().st_size, 'sha256': digest}, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['assemble', 'finalize'])
    if Path(str(TARGET) + '.tar.gz').exists():
        raise ValueError('Published archive already exists; use a new release name')
    {'assemble': assemble, 'finalize': finalize}[parser.parse_args().command]()
