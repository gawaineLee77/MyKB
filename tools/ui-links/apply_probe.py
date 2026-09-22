#!/usr/bin/env python3
"""Exercise the packaged helper in a synthetic instance without starting services."""
from concurrent.futures import ThreadPoolExecutor
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
from configure import sha, TARGET as MOUNT

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'images/archives/mindcreek-r4-20260916-118b8af3-amd64'
ADDON = ROOT / 'images/archives/mindcreek-r4-20260918-links1-amd64-addon'
spec = importlib.util.spec_from_file_location('deployment', BASE / 'bin/mindcreek',
    loader=importlib.machinery.SourceFileLoader('deployment', str(BASE / 'bin/mindcreek')))
deployment = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deployment)

with tempfile.TemporaryDirectory(prefix='ui-links-apply-', dir=ROOT / '.local/ui-links') as directory:
    state = Path(directory)
    deployment.STATE = state
    deployment.initialize('Synthetic-Admin-123!')
    environment = {**os.environ, 'MINDCREEK_STATE_DIR': str(state), 'MINDCREEK_PROJECT': 'mindcreek-ui-links-check'}
    original = {'services': {'gateway': {'image': 'mindcreek-gateway:r4-20260916-graph1'},
        'app': {'environment': {'NEO4J_ENABLE': 'true'}},
        'frontend': {'ports': ['127.0.0.1:18080:80']},
        'tls': {'ports': ['127.0.0.1:14443:443']}}, 'volumes': {'synthetic_graph_data': {}}}
    override = state / 'compose.override.json'
    override.write_text(json.dumps(original))
    override.chmod(0o600)
    env_before = (state / 'enterprise.env').read_bytes()
    def apply(_):
        subprocess.run(['python3', str(ADDON / 'configure.py'), '--package', str(BASE)],
            env=environment, text=True, capture_output=True, check=True)
    with ThreadPoolExecutor(max_workers=2) as pool:
        list(pool.map(apply, range(2)))
    first = override.read_bytes()
    configured = json.loads(first)
    config = state / 'ui-links/links.json'
    assert json.loads(config.read_text())['enabled'] is False
    custom = {'version': 1, 'enabled': True, 'links': {'help': '/internal/help'}}
    config.write_text(json.dumps(custom))
    apply(0)
    assert override.read_bytes() == first and json.loads(config.read_text()) == custom
    for name in ('gateway', 'app', 'tls'):
        assert configured['services'][name] == original['services'][name]
    assert configured['volumes'] == original['volumes']
    assert configured['services']['frontend']['ports'] == original['services']['frontend']['ports']
    assert (state / 'enterprise.env').read_bytes() == env_before
    backups = list(state.glob('compose.override.before-ui-links-*'))
    assert len(backups) == 1 and json.loads(backups[0].read_text()) == original
    assert backups[0].stat().st_mode & 0o777 == 0o600
    assert override.stat().st_mode & 0o777 == 0o600
    assert config.stat().st_mode & 0o777 == 0o644
    rendered = subprocess.run(['python3', str(BASE / 'bin/mindcreek'), 'compose', 'config', '--format', 'json'],
        env=environment, capture_output=True, text=True, check=True)
    compose = json.loads(rendered.stdout)
    frontend = compose['services']['frontend']
    assert frontend['image'] == 'mindcreek-ui:r4-20260918-links1'
    mount = next(item for item in frontend['volumes'] if item['target'] == MOUNT)
    assert mount['source'] == str(state / 'ui-links') and mount['read_only']
    assert compose['services']['gateway']['image'] == original['services']['gateway']['image']
    assert compose['services']['app']['environment']['NEO4J_ENABLE'] == 'true'

proof = {'status': 'passed', 'scope': 'Actual packaged CLI/image import and original Compose rendering; synthetic instance, no services started.',
    'checks': ['concurrent_apply', 'repeat_apply', 'configured_links_preserved', 'other_services_and_env_preserved',
               'single_private_backup', 'public_config_permissions', 'original_compose_valid'],
    'helper_sha256': sha(ADDON / 'configure.py')}
(ROOT / '.local/ui-links/apply.json').write_text(json.dumps(proof, indent=2) + '\n')
print(json.dumps(proof))
