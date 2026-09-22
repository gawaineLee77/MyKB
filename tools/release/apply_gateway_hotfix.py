#!/usr/bin/env python3
"""Apply the packaged gateway image to an existing enterprise instance override."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(8 * 1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


def merge_override(current, patch):
    result = json.loads(json.dumps(current))
    services = result.setdefault('services', {})
    for name in ['gateway', 'installer']:
        service = services.setdefault(name, {})
        if service.get('image') not in [None, patch['base_gateway_image'], patch['image']['name'], *patch.get('replaces', [])]:
            raise ValueError(f'{name} already has a different image override; review it before applying')
        if service.get('platform', 'linux/amd64') != 'linux/amd64':
            raise ValueError(f'{name} has a non-AMD64 override')
        service.update(image=patch['image']['name'], platform='linux/amd64', pull_policy='never')
    return result


def verify_payload(root):
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        expected, relative = line.split('  ', 1)
        candidate = (root / relative).resolve()
        if not candidate.is_relative_to(root.resolve()) or sha(candidate) != expected:
            raise ValueError('Patch checksum mismatch or invalid path: ' + relative)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=Path, required=True, help='Original extracted enterprise bundle')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    verify_payload(root)
    patch = json.loads((root / 'PATCH.json').read_text())
    package = args.package.expanduser().resolve()
    if sha(package / 'RELEASE.json') != patch['base_release_sha256']:
        raise ValueError('This patch requires the exact original enterprise bundle recorded in PATCH.json')
    if not os.environ.get('MINDCREEK_STATE_DIR'):
        raise ValueError('Set MINDCREEK_STATE_DIR to the existing instance directory first')
    state = Path(os.environ['MINDCREEK_STATE_DIR']).expanduser().resolve()
    if not (state / 'enterprise.env').is_file():
        raise ValueError('Existing instance enterprise.env is missing; this tool does not initialize an instance')
    target = state / 'compose.override.json'
    if target.is_symlink():
        raise ValueError('Refusing a symlinked compose override')
    before = target.read_bytes() if target.exists() else None
    current = json.loads(before) if before is not None else {}
    updated = merge_override(current, patch)
    subprocess.run(['docker', 'load', '--input', str(root / 'gateway-image.tar')], check=True)
    wanted = patch['image']
    actual = json.loads(subprocess.check_output([
        'docker', 'image', 'inspect', '--platform', 'linux/amd64', wanted['name']
    ]))[0]
    if (actual['Os'], actual['Architecture']) != ('linux', 'amd64') or actual['Id'] not in [wanted['id'], wanted['config_digest']]:
        raise ValueError('Loaded gateway image does not match the patch lock')
    if current == updated:
        print('This instance already selects the hotfix image; no configuration changed.')
        return
    # Refuse concurrent edits rather than replacing an operator's newer changes.
    if (target.read_bytes() if target.exists() else None) != before:
        raise ValueError('Override changed during image loading; retry after reviewing it')
    backup = None
    if before is not None:
        fd, name = tempfile.mkstemp(prefix='compose.override.before-hotfix-', suffix='.json', dir=state)
        backup = Path(name)
        with os.fdopen(fd, 'wb') as output:
            output.write(before)
    fd, name = tempfile.mkstemp(prefix='.compose-hotfix-', dir=state)
    temporary = Path(name)
    try:
        with os.fdopen(fd, 'w') as output:
            json.dump(updated, output, indent=2)
            output.write('\n')
        temporary.replace(target)
    finally:
        temporary.unlink(missing_ok=True)
    print('Selected the hotfix image for gateway and installer in ' + str(target))
    if backup:
        print('Previous override backup: ' + str(backup))
    print('Original bundle and instance secrets are unchanged. Resume installation or recreate the gateway as described in README_ZH.md.')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError, subprocess.CalledProcessError) as exc:
        raise SystemExit('Patch not applied: ' + str(exc))
