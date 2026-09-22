#!/usr/bin/env python3
"""Configure the optional graph extension for an existing R4 enterprise instance.

No service is restarted and no database or volume is deleted by this command.
The resulting override contains a generated database credential: keep it private.
"""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile

IMAGE = 'mindcreek-neo4j:2025.10.1-graph1'
GATEWAY_IMAGE = 'mindcreek-gateway:r4-20260916-graph1'
SUPPORTED_GATEWAYS = {GATEWAY_IMAGE, 'mindcreek-gateway:r4-20260916-118b8af3',
    'mindcreek-gateway:r4-20260916-short-secret-3959a2e2', 'mindcreek-gateway:r4-20260916-identity-diag1'}
ROUTE_TARGET = '/etc/mindcreek/r3-routes.json'
CAP_TARGET = '/etc/mindcreek/r3-capabilities.json'


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def atomic(path, content):
    if path.is_symlink():
        raise ValueError('Refusing symlink: ' + str(path))
    fd, name = tempfile.mkstemp(prefix='.' + path.name + '-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            stream.write(content)
        Path(name).replace(path)
    finally:
        Path(name).unlink(missing_ok=True)


def graph_service(password):
    return {
        'image': IMAGE, 'platform': 'linux/amd64', 'pull_policy': 'never',
        'restart': 'unless-stopped',
        'labels': {'com.mindcreek.graph': '1'},
        'environment': {
            'NEO4J_AUTH': 'neo4j/' + password,
            'NEO4J_server_memory_heap_initial__size': '512m',
            'NEO4J_server_memory_heap_max__size': '1G',
            'NEO4J_server_memory_pagecache_size': '512m',
            'NEO4J_dbms_security_procedures_allowlist': 'apoc.coll.*,apoc.merge.*,apoc.periodic.*,apoc.version',
            'NEO4J_dbms_security_procedures_unrestricted': 'apoc.periodic.*',
            'NEO4J_server_bolt_enabled': 'true',
            'NEO4J_server_http_enabled': 'false',
            'NEO4J_server_https_enabled': 'false',
            'NEO4J_dbms_usage__report_enabled': 'false',
        },
        'volumes': ['mindcreek_graph_data:/data'],
        'healthcheck': {
            'test': ['CMD-SHELL', 'NEO4J_USERNAME=neo4j NEO4J_PASSWORD="$${NEO4J_AUTH#neo4j/}" cypher-shell -a bolt://localhost:7687 "RETURN apoc.version() AS apoc" >/dev/null 2>&1'],
            'interval': '20s', 'timeout': '20s', 'start_period': '90s', 'retries': 12,
        },
        'logging': {'driver': 'json-file', 'options': {'max-size': '10m', 'max-file': '3'}},
    }


def configure(current, state, password, enabled):
    result = copy.deepcopy(current)
    services = result.setdefault('services', {})
    if 'neo4j' in services and services['neo4j'].get('labels', {}).get('com.mindcreek.graph') != '1':
        raise ValueError('A custom neo4j service exists; do not overwrite it')
    # Preserve operator resource limits on a previously configured graph service.
    if 'neo4j' not in services:
        services['neo4j'] = graph_service(password)
    elif services['neo4j']['image'] != IMAGE or services['neo4j']['environment'].get('NEO4J_AUTH') != 'neo4j/' + password:
        raise ValueError('Graph image or credential differs from the installed extension')
    volumes = result.setdefault('volumes', {})
    volumes.setdefault('mindcreek_graph_data', {})
    app = services.setdefault('app', {}).setdefault('environment', {})
    if app.get('NEO4J_URI', 'bolt://neo4j:7687') != 'bolt://neo4j:7687':
        raise ValueError('A custom Neo4j endpoint exists; review before configuring')
    if app.get('NEO4J_PASSWORD', password) != password or app.get('NEO4J_USERNAME', 'neo4j') != 'neo4j':
        raise ValueError('Custom app graph credentials exist; do not overwrite them')
    app.update(NEO4J_ENABLE=str(enabled).lower(), NEO4J_URI='bolt://neo4j:7687',
               NEO4J_USERNAME='neo4j', NEO4J_PASSWORD=password)
    for name in ['gateway', 'installer']:
        service = services.setdefault(name, {})
        if service.get('image', GATEWAY_IMAGE) not in SUPPORTED_GATEWAYS:
            raise ValueError('Custom gateway image exists; review before configuring')
        service['image'] = GATEWAY_IMAGE
        environment = service.setdefault('environment', {})
        for key, path in [('MINDCREEK_CAPABILITIES_FILE', CAP_TARGET), ('MINDCREEK_NATIVE_ROUTES_FILE', ROUTE_TARGET)]:
            if environment.get(key, path) != path:
                raise ValueError('Custom graph policy path exists; review before configuring')
            environment[key] = path
        environment['MINDCREEK_GRAPH_ENABLED'] = str(enabled).lower()
        for filename, destination in [('capabilities.json', CAP_TARGET), ('routes.json', ROUTE_TARGET)]:
            mount = str(state / ('graph/' + filename)) + ':' + destination + ':ro'
            old = service.setdefault('volumes', [])
            for value in old:
                target = value.get('target') if isinstance(value, dict) else value.split(':')[1]
                if target == destination and value != mount:
                    raise ValueError('A custom graph policy mount exists; review before configuring')
            if mount not in old:
                old.append(mount)
    return result


def verify_payload(root, package):
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        expected, relative = line.split('  ', 1)
        path = (root / relative).resolve()
        if not path.is_relative_to(root.resolve()) or sha(path) != expected:
            raise ValueError('Extension checksum mismatch: ' + relative)
    lock = json.loads((root / 'GRAPH.json').read_text())
    if sha(package / 'RELEASE.json') != lock['base_release_sha256']:
        raise ValueError('This extension requires its recorded R4 enterprise bundle')
    return lock


def load_image(root, lock):
    for archive, image, key in [('neo4j-image.tar', IMAGE, 'image'), ('gateway-image.tar', GATEWAY_IMAGE, 'gateway')]:
        subprocess.run(['docker', 'load', '--input', str(root / archive)], check=True)
        info = json.loads(subprocess.check_output(['docker', 'image', 'inspect', '--platform', 'linux/amd64', image]))[0]
        if (info['Os'], info['Architecture']) != ('linux', 'amd64') or info['Id'] not in [lock[key]['id'], lock[key]['config_digest']]:
            raise ValueError('Image does not match the AMD64 image lock: ' + image)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['enable', 'disable'])
    parser.add_argument('--package', type=Path, required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    package = args.package.expanduser().resolve()
    lock = verify_payload(root, package)
    if not os.environ.get('MINDCREEK_STATE_DIR'):
        raise ValueError('Set MINDCREEK_STATE_DIR to the existing instance directory')
    state = Path(os.environ['MINDCREEK_STATE_DIR']).expanduser().resolve()
    env_path = state / 'enterprise.env'
    if env_path.is_symlink() or not env_path.is_file() or env_path.stat().st_mode & 0o077:
        raise ValueError('Existing private enterprise.env is required')
    # Exact origin avoids the known app/gateway issuer mismatch, without rewriting
    # the company's registered redirect URI or any other authentication setting.
    for line in env_path.read_text().splitlines():
        if line.startswith('MINDCREEK_EXTERNAL_ORIGIN=') and line.split('=', 1)[1].strip().strip('\"\'').endswith('/'):
            raise ValueError('Remove the trailing / from MINDCREEK_EXTERNAL_ORIGIN first')
    graph = state / 'graph'
    if graph.is_symlink():
        raise ValueError('Refusing symlinked graph directory')
    graph.mkdir(mode=0o700, exist_ok=True)
    graph.chmod(0o700)
    guard_path = graph / '.configure.lock'
    if guard_path.is_symlink():
        raise ValueError('Refusing symlinked configuration lock')
    with os.fdopen(os.open(guard_path, os.O_CREAT | os.O_RDWR, 0o600), 'r+') as guard:
        fcntl.flock(guard.fileno(), fcntl.LOCK_EX)
        secret = graph / 'password'
        if not secret.exists():
            if args.action == 'disable':
                raise ValueError('Graph extension has not been initialized')
            atomic(secret, secrets.token_hex(24) + '\n')
        if secret.is_symlink() or secret.stat().st_mode & 0o077:
            raise ValueError('Graph credential must be a private regular file')
        password = secret.read_text().strip()
        if len(password) != 48 or any(c not in '0123456789abcdef' for c in password):
            raise ValueError('Invalid generated graph credential; do not reset a populated database')
        target = state / 'compose.override.json'
        if target.is_symlink():
            raise ValueError('Refusing symlinked compose override')
        before = target.read_bytes() if target.exists() else None
        current = json.loads(before) if before else {}
        updated = configure(current, state, password, args.action == 'enable')
        base_caps = json.loads((package / 'config/r3-capabilities.json').read_text())
        base_caps['capabilities']['rag_graph'] = args.action == 'enable'
        if args.action == 'enable':
            load_image(root, lock)
        if (target.read_bytes() if target.exists() else None) != before:
            raise ValueError('Compose override changed; retry after reviewing it')
        # All writes are repeatable; a crash before completion is recovered by rerun.
        if before is not None and current != updated:
            fd, backup = tempfile.mkstemp(prefix='compose.override.before-graph-', suffix='.json', dir=state)
            with os.fdopen(fd, 'wb') as stream:
                stream.write(before)
            print('Private override backup: ' + backup)
        caps = graph / 'capabilities.json'
        atomic(caps, json.dumps(base_caps, indent=2) + '\n')
        # Capabilities are non-secret and read by the gateway's configured UID.
        caps.chmod(0o644)
        routes = graph / 'routes.json'
        atomic(routes, (root / 'routes.json').read_text())
        routes.chmod(0o644)
        # Directory must be traversable by the product service UID, without listing it.
        graph.chmod(0o711)
        if current != updated:
            atomic(target, json.dumps(updated, indent=2) + '\n')
        else:
            target.chmod(0o600)
        print('Graph ' + args.action + ' configuration prepared; follow README_ZH.md to recreate services.')
        print('Login/model settings and existing volumes are preserved; graph gateway includes the earlier authentication fixes.')



if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError, subprocess.CalledProcessError) as exc:
        # Avoid subprocess output/environment in errors: they can contain secrets.
        raise SystemExit('Graph configuration failed: ' + (str(exc) if not isinstance(exc, subprocess.CalledProcessError) else 'Docker command failed'))
