#!/usr/bin/env python3
"""Install the UI image and merge its runtime link configuration into an existing instance."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

TARGET = '/usr/share/nginx/html/mindcreek-runtime'


def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(8 * 1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


def merge_override(current, state, image):
    result = json.loads(json.dumps(current))
    frontend = result.setdefault('services', {}).setdefault('frontend', {})
    if frontend.get('image') not in (None, image, 'mindcreek-ui:r4-20260916-118b8af3'):
        raise ValueError('Custom frontend image override; review before replacing it')
    if frontend.get('platform', 'linux/amd64') != 'linux/amd64':
        raise ValueError('Frontend override is not AMD64')
    mount = {'type': 'bind', 'source': str(state / 'ui-links'), 'target': TARGET, 'read_only': True}
    volumes = frontend.setdefault('volumes', [])
    exists = False
    for item in volumes:
        target = item.get('target') if isinstance(item, dict) else item.split(':')[1] if ':' in item else ''
        if target == TARGET:
            if item != mount:
                raise ValueError('Custom runtime links mount; review before replacing it')
            exists = True
        elif target in ('/usr/share/nginx/html', '/usr/share/nginx/html/mindcreek-links.json', '/etc/nginx/templates', '/etc/nginx/templates/default.conf.template'):
            raise ValueError('Custom frontend files/template mount; review before applying')
    if not exists:
        volumes.append(mount)
    frontend.update(image=image, platform='linux/amd64', pull_policy='never')
    return result


def atomic_json(path, value, mode=0o600):
    fd, temporary = tempfile.mkstemp(prefix='.' + path.name + '-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            os.fchmod(stream.fileno(), mode)
            json.dump(value, stream, ensure_ascii=False, indent=2)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def verify_payload(root):
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        expected, name = line.split('  ', 1)
        file = (root / name).resolve()
        if not file.is_relative_to(root) or sha(file) != expected:
            raise ValueError('Payload checksum mismatch: ' + name)


def verify_image(image, expected):
    # containerd can return the manifest ID; the classic Docker store returns
    # the config digest. Both are locked from the same exported AMD64 image.
    if (image['Architecture'] != 'amd64' or image['Os'] != 'linux'
            or image['Id'] not in (expected['id'], expected['config_digest'])):
        raise ValueError('Loaded UI image does not match the package')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=Path, required=True, help='Original extracted enterprise bundle')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    verify_payload(root)
    patch = json.loads((root / 'PATCH.json').read_text())
    package = args.package.expanduser().resolve()
    if sha(package / 'RELEASE.json') != patch['base_release_sha256']:
        raise ValueError('This patch requires the recorded R4 enterprise bundle')
    state_value = os.environ.get('MINDCREEK_STATE_DIR', '')
    if not state_value or not Path(state_value).is_absolute():
        raise ValueError('Set MINDCREEK_STATE_DIR to the existing absolute instance directory')
    state = Path(state_value).resolve()
    if not (state / 'enterprise.env').is_file():
        raise ValueError('Existing enterprise.env is required; do not create a new instance')
    lock = os.open(state / '.ui-links.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(lock, 'w') as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        override = state / 'compose.override.json'
        config_dir = state / 'ui-links'
        config_file = config_dir / 'links.json'
        if override.is_symlink() or config_dir.is_symlink() or config_file.is_symlink():
            raise ValueError('Instance override and UI config must not be symlinks')
        current = json.loads(override.read_text()) if override.exists() else {}
        result = merge_override(current, state, patch['image']['name'])
        if config_file.exists():
            existing = json.loads(config_file.read_text())
            if existing.get('version') != 1 or not isinstance(existing.get('enabled'), bool) or not isinstance(existing.get('links'), dict):
                raise ValueError('Existing links.json has an invalid schema')
        subprocess.run(['docker', 'load', '-i', str(root / 'images.tar')], check=True)
        image = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', patch['image']['name']], text=True))[0]
        verify_image(image, patch['image'])
        config_dir.mkdir(mode=0o755, exist_ok=True)
        config_dir.chmod(0o755)  # Public non-secret config must be readable by Nginx.
        if not config_file.exists():
            atomic_json(config_file, json.loads((root / 'links.example.json').read_text()), 0o644)
        config_file.chmod(0o644)
        if current != result:
            if override.exists():
                backup = state / ('compose.override.before-ui-links-' + str(time.time_ns()) + '.json')
                atomic_json(backup, current)
            atomic_json(override, result)
        print('UI image loaded; frontend override prepared. Running services have not been restarted.')
        print('Runtime link configuration: ' + str(config_file))


if __name__ == '__main__':
    main()
