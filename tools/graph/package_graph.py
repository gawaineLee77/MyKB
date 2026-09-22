#!/usr/bin/env python3
"""Assemble an immutable, AMD64-only Neo4j/APOC add-on for the original R4 bundle."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'images/archives/mindcreek-r4-20260916-118b8af3-amd64'
TARGET = ROOT / 'images/archives/mindcreek-r4-20260916-graph1-amd64-addon'
spec = importlib.util.spec_from_file_location('deployment', ROOT / 'deploy/graph/graph.py')
deployment = importlib.util.module_from_spec(spec); spec.loader.exec_module(deployment)
sha = deployment.sha


def checksums():
    (TARGET / 'SHA256SUMS').write_text(''.join(sha(p) + '  ' + str(p.relative_to(TARGET)) + '\n'
        for p in sorted(TARGET.rglob('*')) if p.is_file() and p.name != 'SHA256SUMS' and '__pycache__' not in p.parts))


def export_image(image, filename):
    archive = TARGET / filename
    info = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', image]))[0]
    assert (info['Os'], info['Architecture']) == ('linux', 'amd64')
    previous = json.loads((TARGET / 'GRAPH.json').read_text()) if (TARGET / 'GRAPH.json').exists() else {}
    key = 'image' if filename == 'neo4j-image.tar' else 'gateway'
    if archive.exists() and previous.get(key, {}).get('id', info['Id']) != info['Id']:
        archive.unlink()
    if not archive.exists():
        partial = archive.with_suffix('.partial')
        subprocess.run(['docker', 'image', 'save', '--platform', 'linux/amd64', '--output', str(partial), image], check=True)
        partial.rename(archive)
    with tarfile.open(archive) as tar:
        manifest = json.load(tar.extractfile('manifest.json'))
        assert len(manifest) == 1 and manifest[0]['RepoTags'] == [image]
        raw = tar.extractfile(manifest[0]['Config']).read()
        config = json.loads(raw)
        assert (config['os'], config['architecture']) == ('linux', 'amd64')
        if key == 'gateway':
            found = False
            for layer in manifest[0]['Layers']:
                with tarfile.open(fileobj=tar.extractfile(layer), mode='r:*') as contents:
                    for entry in contents:
                        if entry.name.lstrip('./') == 'mindcreek-gateway' and entry.isfile():
                            digest = hashlib.sha256(contents.extractfile(entry).read()).hexdigest()
                            assert digest == sha(ROOT / '.local/redesign-graph/gateway-linux')
                            found = True
            assert found, 'Gateway executable is missing from exported image'
    return {'name': image, 'id': info['Id'], 'config_digest': 'sha256:' + hashlib.sha256(raw).hexdigest(),
        'os': 'linux', 'architecture': 'amd64'}


def assemble():
    TARGET.mkdir(parents=True, exist_ok=True)
    neo4j = export_image(deployment.IMAGE, 'neo4j-image.tar')
    gateway = export_image(deployment.GATEWAY_IMAGE, 'gateway-image.tar')
    apoc = subprocess.check_output(['docker', 'run', '--rm', '--platform', 'linux/amd64', '--network', 'none',
        '--entrypoint', 'sha256sum', deployment.IMAGE, '/var/lib/neo4j/plugins/apoc.jar'], text=True).split()[0]
    lock = {'release': TARGET.name, 'base_release_sha256': sha(BASE / 'RELEASE.json'),
        'image': neo4j, 'gateway': gateway,
        'gateway_build': json.loads((ROOT / '.local/redesign-graph/gateway-build.json').read_text()),
        'routes_sha256': sha(ROOT / 'config/r3-routes.json'),
        'neo4j_base': 'neo4j:2025.10.1@sha256:155c8aad10d5c838bc3bbc476c0418779086547822acb214ec5e3d49ba336907',
        'apoc_jar_sha256': apoc, 'runtime_downloads': False,
        'scope': 'Optional native graph deployment. Updates gateway/routes with graph previews and earlier OAuth fixes; UI, OAuth and model settings are preserved.'}
    (TARGET / 'GRAPH.json').write_text(json.dumps(lock, indent=2) + '\n')
    for src, dest in [('deploy/graph/graph.py', 'graph.py'), ('config/r3-routes.json', 'routes.json'), ('docs/guides/R4_KNOWLEDGE_GRAPH_ZH.md', 'docs/guides/R4_KNOWLEDGE_GRAPH_ZH.md')]:
        (TARGET / dest).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / src, TARGET / dest)
    (TARGET / 'README_ZH.md').write_text('# MindCreek 原生知识图谱补充包\n\n请从[详细部署和使用指南](docs/guides/R4_KNOWLEDGE_GRAPH_ZH.md)开始。\n\n在本目录执行 graph.py，沿用原企业实例的 MINDCREEK_STATE_DIR 和 MINDCREEK_PROJECT。工具不启动或停止服务。\n')
    (TARGET / 'THIRD_PARTY.md').write_text('''# Third-party runtime

Neo4j Community Edition 2025.10.1 and its bundled APOC Core are preserved in the
image, including upstream license notices. The image adds the bundled JAR to the
plugin directory; it does not download plugins at startup.

Sources: https://github.com/neo4j/neo4j and https://github.com/neo4j/apoc
Plugin deployment: https://neo4j.com/docs/operations-manual/current/docker/plugins/
''')
    checksums()
    print(TARGET)


def finalize():
    proof = ROOT / 'docs/plans/evidence/graph-release.json'
    data = json.loads(proof.read_text())
    assert data['status'] == 'passed'
    assert data['graph_helper_sha256'] == sha(ROOT / 'deploy/graph/graph.py') == sha(TARGET / 'graph.py')
    assert data['gateway_image_id'] == json.loads((TARGET / 'GRAPH.json').read_text())['gateway']['id']
    assert data['routes_sha256'] == sha(ROOT / 'config/r3-routes.json') == sha(TARGET / 'routes.json')
    assert data['image_id'] == json.loads((TARGET / 'GRAPH.json').read_text())['image']['id']
    assert (TARGET / 'docs/guides/R4_KNOWLEDGE_GRAPH_ZH.md').read_bytes() == (ROOT / 'docs/guides/R4_KNOWLEDGE_GRAPH_ZH.md').read_bytes()
    paths = [ROOT / 'docs/plans/R4_KNOWLEDGE_GRAPH_ACCEPTANCE.md',
        *sorted((ROOT / 'docs/plans/evidence').glob('graph-*.json')),
        *[ROOT / p for p in json.loads((ROOT / 'docs/plans/evidence/graph-browser.json').read_text())['screenshots']]]
    for path in paths:
        target = TARGET / path.relative_to(ROOT)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, target)
    shutil.copy2(proof, TARGET / 'evidence.json')
    checksums()
    archive = Path(str(TARGET) + '.tar.gz')
    if archive.exists(): raise ValueError('Archive exists; refusing overwrite')
    partial = Path(str(archive) + '.partial')
    with tarfile.open(partial, 'w:gz', compresslevel=1) as tar:
        tar.add(TARGET, arcname=TARGET.name, filter=lambda entry: None if '__pycache__' in entry.name else entry)
    partial.rename(archive)
    digest = sha(archive)
    Path(str(archive) + '.sha256').write_text(digest + '  ' + archive.name + '\n')
    print(json.dumps({'path': str(archive), 'bytes': archive.stat().st_size, 'sha256': digest}, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['assemble', 'finalize'])
    {'assemble': assemble, 'finalize': finalize}[parser.parse_args().command]()
