#!/usr/bin/env python3
"""Validate and build a source-bound AMD64 release; keep previous evidence intact."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess

import build_enterprise as builder
from r5_evidence import ui_digest
from r4_evidence import source_digest

ROOT = Path(__file__).resolve().parents[2]


def workdir():
    return Path(os.environ.get('MINDCREEK_CURRENT_WORKDIR', ROOT / '.local/enterprise-current')).resolve()


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def bundle_digest(directory):
    digest = hashlib.sha256()
    files = sorted(p for p in Path(directory).rglob('*') if p.is_file())
    if not files:
        raise RuntimeError('Missing compiled frontend artifact')
    for path in files:
        digest.update(str(path.relative_to(directory)).encode() + b'\0' + path.read_bytes())
    return digest.hexdigest()


def inputs_digest():
    """Bind runtime and validation code, excluding generated reports and docs."""
    digest = hashlib.sha256()
    paths = []
    for base in ('deploy/enterprise', 'deploy/graph', 'deploy/phase5', 'config', 'images/mindcreek-ui',
                 'images/mindcreek-gateway'):
        paths.extend(p for p in (ROOT / base).rglob('*') if p.is_file()
                     and '__pycache__' not in p.parts and p.suffix != '.md')
    paths.extend((ROOT / 'tools/release').glob('*.py'))
    paths.extend(ROOT / name for name in ('scripts/render-phase5-models.py', 'Makefile'))
    for path in sorted(paths):
        digest.update(str(path.relative_to(ROOT)).encode() + b'\0' + path.read_bytes() + b'\0')
    return digest.hexdigest()


def migration_version():
    migrations = ROOT / 'services/gateway/internal/database/migrations'
    versions = [int(path.name.split('_', 1)[0]) for path in migrations.glob('*.up.sql')]
    if not versions:
        raise RuntimeError('Missing product migrations')
    return max(versions)


def fingerprints():
    return {'gateway_source_sha256': source_digest(), 'ui_source_sha256': ui_digest(),
            'release_inputs_sha256': inputs_digest(),
            'product_migration_version': migration_version(),
            'upstream_commit': subprocess.check_output(
                ['git', '-C', str(ROOT / 'upstream/weknora'), 'rev-parse', 'HEAD'], text=True).strip()}


def frontend_count(log):
    values = {}
    for field in ('tests', 'pass', 'fail', 'cancelled', 'skipped', 'todo'):
        matches = re.findall(r'^(?:ℹ|#)\s+' + field + r'\s+(\d+)\s*$', log, re.M)
        if len(matches) != 1:
            raise RuntimeError('Missing or ambiguous frontend summary: ' + field)
        values[field] = int(matches[0])
    if not values['tests'] or values['pass'] != values['tests'] or any(values[k] for k in ('fail', 'cancelled', 'skipped', 'todo')):
        raise RuntimeError('Frontend suite is incomplete')
    return values['tests']


def installer_count(log):
    matches = re.findall(r'^Ran (\d+) tests? in ', log, re.M)
    if len(matches) != 1 or not re.search(r'^OK\s*$', log, re.M) or int(matches[0]) < 28:
        raise RuntimeError('Installer suite is incomplete')
    return int(matches[0])


def check_upstream():
    if subprocess.check_output(['git', '-C', str(ROOT / 'upstream/weknora'), 'status', '--porcelain'], text=True).strip():
        raise RuntimeError('Pinned upstream must remain clean')


def validate():
    local = workdir()
    local.mkdir(parents=True, exist_ok=True)
    node = subprocess.check_output(['node', '--version'], text=True).strip()
    go = subprocess.check_output(['go', 'version'], text=True).strip()
    if not node.startswith('v24.') or not re.search(r'\bgo1\.26\.', go):
        raise RuntimeError('Validation requires Node 24 and Go 1.26 on PATH')
    check_upstream()
    before = fingerprints()
    run = local / ('validation-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(4))
    run.mkdir()
    upstream = ROOT / 'upstream/weknora/frontend'
    if not (upstream / 'node_modules').is_dir():
        raise RuntimeError('Offline validation needs installed upstream frontend dependencies')
    ui = run / 'ui'
    shutil.copytree(upstream, ui, ignore=shutil.ignore_patterns('node_modules', 'dist', '.env*'))
    (ui / 'node_modules').symlink_to(upstream / 'node_modules', target_is_directory=True)
    logs = {}

    def execute(name, command, cwd=ROOT, env=None):
        path = run / name
        print('Validating: ' + name, flush=True)
        with path.open('w') as out:
            subprocess.run(command, cwd=cwd, env=env, stdout=out, stderr=subprocess.STDOUT, check=True)
        logs[name] = {'path': str(path.relative_to(local)), 'sha256': sha(path)}

    execute('overlay.log', ['node', str(ROOT / 'tools/frontend-overlay/apply.mjs'), str(ui), str(ROOT / 'branding/mindcreek')])
    execute('frontend-tests.log', ['npm', 'test'], cwd=ui)
    count = frontend_count((run / 'frontend-tests.log').read_text())
    execute('typecheck.log', ['npm', 'run', 'type-check'], cwd=ui)
    execute('build.log', ['npm', 'run', 'build'], cwd=ui, env={**os.environ, 'VITE_IS_DOCKER': 'true'})
    execute('native-boundary.log', ['node', str(ROOT / 'tools/frontend-overlay/check-native.mjs'), str(ui), str(upstream)])
    execute('gateway-tests.log', ['make', 'phase1-gateway-test'])
    execute('installer-python312.log', ['docker', 'run', '--rm', '--pull', 'never', '--platform', 'linux/amd64',
            '--network', 'none', '-v', str(ROOT) + ':/src:ro', '-w', '/src', 'python:3.12-alpine',
            'python', '-B', '-m', 'unittest', 'discover', '-s', 'tools/release', '-p', 'test_*.py'])
    python_count = installer_count((run / 'installer-python312.log').read_text())
    if fingerprints() != before:
        raise RuntimeError('Sources changed during validation; rerun before building')
    check_upstream()
    proof = {'status': 'passed', **before, 'node': node, 'go': go, 'python': '3.12',
             'created_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
             'ui_path': str(ui), 'bundle_sha256': bundle_digest(ui / 'dist'),
             'frontend_tests': count, 'python312_tests': python_count, 'logs': logs}
    (local / 'validation.json').write_text(json.dumps(proof, indent=2) + '\n')
    (local / 'ui-path').write_text(str(ui) + '\n')
    (local / 'ui-build.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(f'Current validation passed: {count} frontend and {python_count} installer tests; {run}', flush=True)
    return proof


def validated_proof(local=None):
    local = local or workdir()
    proof = json.loads((local / 'validation.json').read_text())
    if proof.get('status') != 'passed' or any(proof.get(k) != v for k, v in fingerprints().items()):
        raise RuntimeError('Validation is missing or stale for current sources; run build_current.py validate')
    required = {'overlay.log', 'frontend-tests.log', 'typecheck.log', 'build.log', 'native-boundary.log',
                'gateway-tests.log', 'installer-python312.log'}
    if set(proof.get('logs', {})) != required:
        raise RuntimeError('Incomplete validation logs')
    for name, entry in proof['logs'].items():
        path = (local / entry['path']).resolve()
        if not path.is_relative_to(local.resolve()) or sha(path) != entry['sha256']:
            raise RuntimeError('Validation log changed: ' + name)
    frontend = frontend_count((local / proof['logs']['frontend-tests.log']['path']).read_text())
    installer = installer_count((local / proof['logs']['installer-python312.log']['path']).read_text())
    if (frontend, installer) != (proof.get('frontend_tests'), proof.get('python312_tests')):
        raise RuntimeError('Validation summary changed')
    if bundle_digest(Path(proof['ui_path']) / 'dist') != proof['bundle_sha256']:
        raise RuntimeError('Validated frontend artifact changed')
    check_upstream()
    return proof


def build():
    local = workdir()
    proof = validated_proof(local)
    builder.LOCAL = local
    builder.EXTERNAL = [*builder.EXTERNAL, 'mindcreek-neo4j:2025.10.1-graph1']
    builder.build('current', local / 'ui-build.json', Path(proof['ui_path']))
    record = json.loads((local / 'build.json').read_text())
    record.update(release_inputs_sha256=proof['release_inputs_sha256'], validation_sha256=sha(local / 'validation.json'),
                  product_migration_version=proof['product_migration_version'])
    (local / 'build.json').write_text(json.dumps(record, indent=2) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', nargs='?', choices=('validate', 'build', 'all'), default='all')
    args = parser.parse_args()
    if args.command in ('validate', 'all'):
        validate()
    if args.command in ('build', 'all'):
        build()
