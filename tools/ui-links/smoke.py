"""Exercise the real AMD64 Nginx runtime with disposable, non-secret link config."""
import json
from pathlib import Path
import subprocess
import tempfile
import uuid
from configure import atomic_json

root = Path(__file__).resolve().parents[2]
image = 'mindcreek-ui:r4-20260918-links1'
name = 'mindcreek-ui-links-' + uuid.uuid4().hex[:10]

def run(*args):
    return subprocess.check_output(['docker', *args], text=True, stderr=subprocess.STDOUT)

with tempfile.TemporaryDirectory(prefix='mindcreek-ui-links-') as directory:
    Path(directory).chmod(0o755)
    config = Path(directory) / 'links.json'
    try:
        run('run', '-d', '--name', name, '--platform', 'linux/amd64', '--network', 'none',
            '-e', 'APP_HOST=127.0.0.1', '-e', 'APP_PORT=9', '-e', 'APP_SCHEME=http',
            '--mount', f'type=bind,source={directory},target=/usr/share/nginx/html/mindcreek-runtime,readonly', image)
        # nginx config validation is an explicit readiness probe, without API calls.
        run('exec', name, 'sh', '-c', 'for n in 1 2 3 4 5; do wget -q -O /tmp/links-response http://127.0.0.1/mindcreek-links.json && exit 0; sleep 1; done; exit 1')
        run('exec', name, 'nginx', '-t')
        def get():
            return run('exec', name, 'wget', '-S', '-O', '-', 'http://127.0.0.1/mindcreek-links.json')
        def body(response):
            return json.JSONDecoder().raw_decode(response[response.index('{'):])[0]
        original = get()
        assert 'Cache-Control: no-store' in original
        assert body(original)['enabled'] is False
        configured = {'version': 1, 'enabled': True, 'links': {'help': 'https://docs.example.invalid/guide'}}
        atomic_json(config, configured, 0o644)
        live = get()
        assert body(live) == configured
        configured['links']['help'] = '/internal-docs/updated'
        atomic_json(config, configured, 0o644)
        updated = get()
        assert body(updated) == configured
        config.unlink()
        fallback = get()
        assert body(fallback)['enabled'] is False
        info = json.loads(run('image', 'inspect', '--platform', 'linux/amd64', image))[0]
        report = {'status': 'passed', 'image': {'name': image, 'id': info['Id'], 'architecture': info['Architecture']},
                  'checks': ['nginx_config_valid', 'default_hidden', 'no_store', 'mounted_config', 'atomic_live_update', 'missing_mount_file_hidden'],
                  'scope': 'Actual Linux AMD64 Nginx image on ARM64 Docker emulation; network disabled, synthetic public URLs.'}
        (root / '.local/ui-links/nginx.json').write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps(report))
    finally:
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
