#!/usr/bin/env python3
"""Exercise the shipped extension with an isolated synthetic instance; no service starts."""
from concurrent.futures import ThreadPoolExecutor
import hashlib
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PACKAGE = ROOT / 'images/archives/mindcreek-r4-20260916-118b8af3-amd64'
ADDON = ROOT / 'images/archives/mindcreek-r4-20260916-graph1-amd64-addon'
spec = importlib.util.spec_from_file_location('deployment', PACKAGE / 'bin/mindcreek',
    loader=importlib.machinery.SourceFileLoader('deployment', str(PACKAGE / 'bin/mindcreek')))
deployment = importlib.util.module_from_spec(spec); spec.loader.exec_module(deployment)

with tempfile.TemporaryDirectory(prefix='graph-apply-', dir=ROOT / '.local/graph') as directory:
    state = Path(directory)
    deployment.STATE = state
    deployment.initialize('Synthetic-Admin-123!')
    environment = {**os.environ, 'MINDCREEK_STATE_DIR': str(state), 'MINDCREEK_PROJECT': 'mindcreek-graph-config-check'}
    original = {'services': {'gateway': {'image': 'mindcreek-gateway:r4-20260916-identity-diag1'},
        'installer': {'image': 'mindcreek-gateway:r4-20260916-identity-diag1'},
        'tls': {'ports': ['127.0.0.1:14443:443']}}}
    override = state / 'compose.override.json'
    override.write_text(json.dumps(original)); override.chmod(0o600)
    env_before = (state / 'enterprise.env').read_bytes()
    def apply(action):
        subprocess.run(['python3', str(ADDON / 'graph.py'), action, '--package', str(PACKAGE)],
            env=environment, text=True, capture_output=True, check=True)
    with ThreadPoolExecutor(max_workers=2) as pool:
        list(pool.map(apply, ['enable', 'enable']))
    first = override.read_bytes()
    password = (state / 'graph/password').read_bytes()
    apply('enable'); assert override.read_bytes() == first
    enabled = json.loads(first)
    assert enabled['services']['gateway']['image'] == 'mindcreek-gateway:r4-20260916-graph1'
    assert enabled['services']['tls'] == original['services']['tls']
    apply('disable')
    assert json.loads((state / 'graph/capabilities.json').read_text())['capabilities']['rag_graph'] is False
    assert json.loads(override.read_text())['services']['app']['environment']['NEO4J_ENABLE'] == 'false'
    apply('enable')
    assert override.read_bytes() == first and (state / 'graph/password').read_bytes() == password
    assert (state / 'enterprise.env').read_bytes() == env_before
    assert override.stat().st_mode & 0o777 == 0o600
    assert (state / 'graph/password').stat().st_mode & 0o777 == 0o600
    assert all(p.stat().st_mode & 0o777 == 0o600 for p in state.glob('compose.override.before-graph-*'))
    rendered = subprocess.run(['python3', str(PACKAGE / 'bin/mindcreek'), 'compose', 'config', '--format', 'json'],
        env=environment, capture_output=True, text=True, check=True)
    compose = json.loads(rendered.stdout)
    assert compose['services']['app']['environment']['NEO4J_ENABLE'] == 'true'
    assert 'NEO4J_AUTH#neo4j/' in compose['services']['neo4j']['healthcheck']['test'][1]
    assert password.decode().strip() not in compose['services']['neo4j']['healthcheck']['test'][1]
    assert not compose['services']['neo4j'].get('ports')
    assert compose['services']['neo4j']['environment']['NEO4J_AUTH'] == 'neo4j/' + password.decode().strip()
    volumes = compose['services']['gateway']['volumes']
    assert next(v for v in volumes if v['target'] == '/etc/mindcreek/r3-capabilities.json')['source'] == str(state / 'graph/capabilities.json')

proof = {'status': 'passed', 'scope': 'Actual packaged CLI/image load, synthetic private instance, no business services started',
    'concurrent_enable': True, 'repeat_enable': True, 'disable_enable_preserves_password_and_volume': True,
    'gateway_updated_tls_and_env_preserved': True, 'private_backups': True,
    'original_compose_merged_and_valid': True, 'neo4j_ports_private': True,
    'graph_helper_sha256': hashlib.sha256((ADDON / 'graph.py').read_bytes()).hexdigest()}
(ROOT / '.local/graph/apply-evidence.json').write_text(json.dumps(proof, indent=2) + '\n')
print('PASS packaged graph configure, concurrent/repeat enable, disable/recovery and Compose merge')
