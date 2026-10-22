#!/usr/bin/env python3
"""Offline release maintenance: plan, cold backup, upgrade and isolated recovery.

No live deployment is selected implicitly. All operations require an explicit
absolute state directory and project name. Backups contain secrets.
"""
from __future__ import annotations
import argparse
import base64
import contextlib
import datetime
import fcntl
import hashlib
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
UPSTREAM = '1edcd54b43606d9079bb36650efe3f68707a79ea'
GRAPH_IMAGE = 'mindcreek-neo4j:2025.10.1-graph1'
OLD_GATEWAYS = {'mindcreek-gateway:' + v for v in (
    'r4-20260916-118b8af3', 'r4-20260916-short-secret-3959a2e2',
    'r4-20260916-identity-diag1', 'r4-20260916-graph1')}
OLD_UIS = {'mindcreek-ui:r4-20260916-118b8af3', 'mindcreek-ui:r4-20260918-links1'}
POLICIES = {'/etc/mindcreek/r3-routes.json': 'routes.json',
            '/etc/mindcreek/r3-capabilities.json': 'capabilities.json'}
DELEGATION_KEY_SETTING = 'MINDCREEK_WORKSPACE_ADMIN_SECRET_FILE'
DELEGATION_KEY_PATH = '/run/mindcreek-enterprise/workspace-admin-key'
HISTORICAL_MIGRATIONS = {'r4-20260916-118b8af3': 15, 'current-20261009-a86981ad': 16}


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(8 * 1024 * 1024), b''): h.update(block)
    return h.hexdigest()


def atomic_json(path, data):
    if path.is_symlink(): raise ValueError('Refusing symlink: ' + str(path))
    fd, name = tempfile.mkstemp(prefix='.' + path.name, dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            json.dump(data, stream, ensure_ascii=False, indent=2)
            stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
        os.replace(name, path)
    finally:
        Path(name).unlink(missing_ok=True)


def delegation_configuration(dep, apply=False):
    """Prepare missing installation encryption material without rotating a key.

    Plan is read-only. Upgrade applies this first to its isolated preflight copy,
    then to the actual instance only after the old version's cold backup.
    """
    if int(dep.release().get('product_migration_version', 0)) < 17:
        return []
    values = dep.read_env()
    configured = values.get(DELEGATION_KEY_SETTING, '')
    if configured and configured != DELEGATION_KEY_PATH:
        raise ValueError('Review custom workspace delegation key path before upgrade')
    directory = dep.STATE / 'secrets'
    if directory.is_symlink() or not directory.is_dir():
        raise ValueError('Workspace delegation secrets must use the existing regular directory')
    path = directory / 'workspace-admin-key'
    changes = []
    if path.is_symlink():
        raise ValueError('Workspace delegation key must not be a symlink')
    if path.exists():
        dep.private(path)
        if len(base64.b64decode(path.read_text().strip(), validate=True)) != 32:
            raise ValueError('Existing workspace delegation key is invalid; it will not be replaced')
        if path.stat().st_uid != int(values['MINDCREEK_UID']):
            raise ValueError('MINDCREEK_UID must own the existing workspace delegation key')
    elif configured:
        # A configured but lost key might protect existing encrypted sessions.
        raise ValueError('Configured workspace delegation key is missing; restore it from a matching backup')
    else:
        if os.getuid() != int(values['MINDCREEK_UID']):
            raise ValueError('Run upgrade as MINDCREEK_UID to create the workspace delegation key')
        changes.append('create private workspace delegation encryption key')
        if apply:
            with open(path, 'x', opener=lambda name, flags: os.open(name, flags | os.O_NOFOLLOW, 0o600)) as stream:
                stream.write(base64.b64encode(secrets.token_bytes(32)).decode() + '\n')
                stream.flush(); os.fsync(stream.fileno())
    if not configured:
        changes.append('configure workspace delegation encryption file')
        if apply:
            env = dep.STATE / 'enterprise.env'
            dep.private(env)
            content = env.read_text()
            line = DELEGATION_KEY_SETTING + '=' + DELEGATION_KEY_PATH
            pattern = r'(?m)^[ \t]*' + DELEGATION_KEY_SETTING + r'[ \t]*=[^\r\n]*'
            if re.search(pattern, content):
                content = re.sub(pattern, lambda _: line, content)
            else:
                content += ('' if content.endswith('\n') else '\n') + line + '\n'
            fd, temporary = tempfile.mkstemp(prefix='.enterprise.env.', dir=dep.STATE)
            try:
                with os.fdopen(fd, 'w') as stream:
                    stream.write(content); stream.flush(); os.fsync(stream.fileno())
                os.replace(temporary, env)
            finally:
                Path(temporary).unlink(missing_ok=True)
    return changes


def access_review(release):
    if int(release.get('product_migration_version', 0)) < 18:
        return []
    return [
        'Owner inventories historical knowledge bases; default visibility becomes restricted to creator and Owner/Admin',
        'Owner explicitly grants public/member visibility and document/chunk operations; upgrade never publishes knowledge bases',
        'Owner reviews legacy API keys and saves explicit product knowledge-base scope before integrations resume',
        'Owner enables separate workspace administration and knowledge collaboration delegations as needed',
        'Verify workspace memory is disabled and existing agents use supported read-only configurations',
        'Verify Owner/Admin/Contributor/Viewer, retrieval, downloads, revocation and Wiki MCP before reopening traffic',
    ]


def module(path, name):
    loader = importlib.machinery.SourceFileLoader(name, str(path))
    spec = importlib.util.spec_from_loader(name, loader)
    result = importlib.util.module_from_spec(spec); loader.exec_module(result)
    return result


def deployment(package):
    return module(package / 'bin/mindcreek', 'deployment_' + hashlib.sha256(str(package).encode()).hexdigest()[:8])


def selection():
    state = os.environ.get('MINDCREEK_STATE_DIR', '')
    project = os.environ.get('MINDCREEK_PROJECT', '')
    if not Path(state).is_absolute() or not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', project):
        raise ValueError('Set an absolute MINDCREEK_STATE_DIR and explicit MINDCREEK_PROJECT')
    if Path(state).is_symlink(): raise ValueError('State directory must not be a symlink')
    return Path(state).resolve(), project


@contextlib.contextmanager
def lock(state):
    fd = os.open(state / '.release.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as stream:
        fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield


def mount_parts(mount):
    if isinstance(mount, dict): return mount.get('source'), mount.get('target')
    parts = mount.split(':')
    return (parts[0], parts[1]) if len(parts) >= 2 else (None, parts[0])


def override_plan(current, state, old, new):
    """Remove only known product pins; preserve operator configuration or fail closed."""
    result = json.loads(json.dumps(current)); changes = []
    services = result.setdefault('services', {})
    allowed = {i['name'] for i in old['images']} | OLD_GATEWAYS | OLD_UIS
    for name in ('gateway', 'installer', 'frontend'):
        service = services.get(name, {})
        if 'image' in service:
            if service['image'] not in allowed:
                raise ValueError('Review custom image override before upgrade: ' + name)
            del service['image']; changes.append('remove released image pin: ' + name)
        kept = []
        for mount in service.get('volumes', []):
            source, target = mount_parts(mount)
            if target in POLICIES:
                expected = str(state / 'graph' / POLICIES[target])
                if source != expected: raise ValueError('Review custom policy mount: ' + name)
                # Retain its path: configuration is regenerated from the new package.
                changes.append('refresh graph policy: ' + POLICIES[target])
            if name == 'frontend' and target in ('/usr/share/nginx/html', '/etc/nginx/templates', '/etc/nginx/templates/default.conf.template'):
                raise ValueError('Review custom frontend mount before upgrade')
            kept.append(mount)
        if 'volumes' in service: service['volumes'] = kept
    if 'neo4j' in services:
        neo = services['neo4j']
        if neo.get('image') != GRAPH_IMAGE or neo.get('labels', {}).get('com.mindcreek.graph') != '1':
            raise ValueError('Custom Neo4j requires a reviewed upgrade; data is not changed')
        env = services.get('app', {}).get('environment', {})
        enabled = str(env.get('NEO4J_ENABLE', 'false')).lower() == 'true'
        if enabled:
            password = env.get('NEO4J_PASSWORD')
            if not password or neo.get('environment', {}).get('NEO4J_AUTH') != 'neo4j/' + password:
                raise ValueError('Existing graph credentials do not match; do not regenerate them')
            graph = services.setdefault('gateway', {}).setdefault('environment', {})
            for k, v in {'MINDCREEK_GRAPH_VIEW_URI': 'bolt://neo4j:7687',
                         'MINDCREEK_GRAPH_VIEW_USERNAME': 'neo4j',
                         'MINDCREEK_GRAPH_VIEW_PASSWORD': password,
                         'MINDCREEK_GRAPH_VIEW_DATABASE': 'neo4j'}.items():
                if graph.get(k, v) != v: raise ValueError('Review custom graph-view configuration: ' + k)
                graph[k] = v
            changes.append('enable read-only browser for existing graph')
    return result, sorted(set(changes))


def checked_source(old, new):
    if old.get('upstream_commit') != UPSTREAM or new.get('upstream_commit') != UPSTREAM:
        raise ValueError('Only the pinned WeKnora v0.8.0 enterprise runtime can upgrade in place; legacy Phase 5 needs a separate migration')
    if old.get('platform') != 'linux/amd64': raise ValueError('Source release must be AMD64')
    versions = []
    for release in (old, new):
        version = release.get('product_migration_version', HISTORICAL_MIGRATIONS.get(release.get('release')))
        if type(version) is not int or version < 1:
            raise ValueError('Unknown product migration version; review this release before upgrading')
        versions.append(version)
    if versions[0] > versions[1]:
        raise ValueError('In-place downgrade is forbidden; restore the matching cold backup into an isolated instance')
    for prefix in ('paradedb/', 'redis:', 'wechatopenai/weknora-app:', 'wechatopenai/weknora-docreader:'):
        before = next((i for i in old['images'] if i['name'].startswith(prefix)), None)
        after = next((i for i in new['images'] if i['name'].startswith(prefix)), None)
        if not before or not after or before['id'] != after['id']:
            raise ValueError('Dependency change needs a separate migration review: ' + prefix)


def resolved(dep):
    return json.loads(dep.compose(['--profile', 'tools', '--profile', 'tls', 'config', '--format', 'json'], True))


def containers(project):
    ids = subprocess.check_output(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=' + project], text=True).split()
    return json.loads(subprocess.check_output(['docker', 'inspect', *ids], text=True)) if ids else []


def volumes_for(dep, config):
    volumes = {}
    for logical, spec in config.get('volumes', {}).items():
        name = spec.get('name', dep.PROJECT + '_' + logical)
        probe = subprocess.run(['docker', 'volume', 'inspect', name], text=True, capture_output=True)
        if probe.returncode: continue
        info = json.loads(probe.stdout)[0]
        if spec.get('external') or (info.get('Labels') or {}).get('com.docker.compose.project') != dep.PROJECT:
            raise ValueError('External or differently owned volume requires separate backup: ' + logical)
        volumes[logical] = name
    if not {'postgres-data', 'redis-data', 'data-files'} <= volumes.keys():
        raise ValueError('Existing enterprise database, Redis and files volumes are required; check project name')
    return volumes


def confirm_instance(dep, config):
    live = containers(dep.PROJECT)
    if not live: raise ValueError('No containers in this project; cannot identify an existing deployment')
    # The gateway secret bind proves which instance directory belongs to the project.
    gateway = [c for c in live if c.get('Config', {}).get('Labels', {}).get('com.docker.compose.service') == 'gateway']
    expected = str(dep.STATE / 'secrets')
    if len(gateway) != 1 or not any(m.get('Source') == expected and m.get('Destination') == '/run/mindcreek-enterprise' for m in gateway[0]['Mounts']):
        raise ValueError('Project and state do not identify the same gateway instance')
    configured = set(config['services'])
    if any(c['Config']['Labels'].get('com.docker.compose.service') not in configured for c in live):
        raise ValueError('Unconfigured project containers exist; include them before backup')
    for service in config['services'].values():
        for mount in service.get('volumes', []):
            if mount.get('type') != 'bind' or mount.get('read_only'): continue
            source = Path(mount['source']).resolve()
            if not source.is_relative_to(dep.STATE):
                raise ValueError('External writable bind requires a separate backup: ' + str(source))
    return live


def plan(old_package, dep):
    old = deployment(old_package)
    checked_source(old.release(), dep.release())
    old.read_env(); dep.read_env()
    override = dep.STATE / 'compose.override.json'
    if override.is_symlink(): raise ValueError('Override must not be a symlink')
    current = json.loads(override.read_text()) if override.exists() else {}
    updated, changes = override_plan(current, dep.STATE, old.release(), dep.release())
    configuration_changes = delegation_configuration(dep)
    config = resolved(old); confirm_instance(old, config)
    volumes = volumes_for(old, config)
    return old, updated, {'from_release': old.release()['release'], 'to_release': dep.release()['release'],
        'project': dep.PROJECT, 'state': str(dep.STATE), 'volumes': volumes, 'changes': changes,
        'configuration_changes': configuration_changes,
        'access_review_required': access_review(dep.release()),
        'database': 'preserve native/product schemas; gateway applies forward-only pending product migrations',
        'recovery': 'cold backup of all named volumes and instance configuration; restore into a new isolated project'}


# This program runs inside the bundled Python ops image, with no network access.
VOLUME_IO = r'''
import os,pathlib,shutil,tarfile,sys
mode=sys.argv[1]
for name in sys.argv[2:]:
 root=pathlib.Path('/volumes')/name; archive=pathlib.Path('/backup')/(name+'.tar.gz')
 if mode=='save':
  def allowed(item):
   if item.issym() or item.islnk() or not (item.isfile() or item.isdir()):raise ValueError('Unsupported live volume entry')
   return item
  with tarfile.open(archive,'w:gz',compresslevel=1) as tar:tar.add(root,arcname='data',filter=allowed)
 else:
  if any(root.iterdir()):raise ValueError('Restore volume is not empty')
  with tarfile.open(archive) as tar:
   for member in tar:
    parts=pathlib.PurePosixPath(member.name).parts
    if not parts or parts[0]!='data' or '..' in parts or member.issym() or member.islnk():raise ValueError('Unsafe backup member')
    dest=root.joinpath(*parts[1:])
    if member.isdir():dest.mkdir(parents=True,exist_ok=True)
    elif member.isfile():
     dest.parent.mkdir(parents=True,exist_ok=True)
     with tar.extractfile(member) as src,dest.open('wb') as out:shutil.copyfileobj(src,out)
    else:raise ValueError('Unsupported backup entry')
    os.chown(dest,member.uid,member.gid);os.chmod(dest,member.mode & 0o777)
'''


def volume_io(dep, destination, volumes, mode):
    image = next(i['name'] for i in dep.release()['images'] if i['name'].startswith('python:'))
    cmd = ['docker', 'run', '--rm', '--pull', 'never', '--platform', 'linux/amd64', '--network', 'none',
           '-v', str(destination) + ':/backup' + (':ro' if mode == 'restore' else ':rw')]
    for key, name in volumes.items():
        if not re.fullmatch(r'[a-zA-Z0-9_-]+', key): raise ValueError('Invalid volume key')
        cmd += ['-v', name + ':/volumes/' + key + (':ro' if mode == 'save' else ':rw')]
    subprocess.run([*cmd, image, 'python', '-c', VOLUME_IO, mode, *volumes], check=True)


def backup(dep, destination):
    if destination.exists(): raise ValueError('Backup destination must be new')
    config = resolved(dep); live = confirm_instance(dep, config)
    volumes = volumes_for(dep, config)
    running = [c['Config']['Labels']['com.docker.compose.service'] for c in live if c['State']['Running']]
    destination.mkdir(mode=0o700, parents=True)
    atomic_json(destination / 'INCOMPLETE.json', {'project': dep.PROJECT, 'state': str(dep.STATE)})
    dep.compose(['--profile', 'tls', '--profile', 'tools', 'stop', '--timeout', '120'])
    if any(c['State']['Running'] for c in containers(dep.PROJECT)):
        raise ValueError('Containers are still running; backup aborted')
    volume_io(dep, destination, volumes, 'save')
    # Add-on image tags may not be present in the original full-package tar.
    # Preserve the actual UI/gateway images needed by its private override.
    products = sorted({c['Config']['Image'] for c in live
                       if c['Config']['Image'].startswith(('mindcreek-ui:', 'mindcreek-gateway:'))})
    if products:
        subprocess.run(['docker', 'image', 'save', '--platform', 'linux/amd64',
                        '--output', str(destination/'rollback-product-images.tar'), *products], check=True)
    def include(info):
        p = Path(info.name)
        if len(p.parts) > 1 and p.parts[1] == 'backup': return None
        if p.name == '.release.lock': return None
        if info.issym() or info.islnk() or not (info.isfile() or info.isdir()):
            raise ValueError('Instance backup refuses symlinks and special files')
        return info
    # Backups must be outside state, or under its reserved backup directory.
    with tarfile.open(destination / 'instance.tar.gz', 'w:gz', compresslevel=1) as tar:
        tar.add(dep.STATE, arcname='instance', filter=include)
    metadata = {'version': 1, 'release': dep.release(), 'project': dep.PROJECT, 'state': str(dep.STATE),
                'volumes': volumes, 'running_services': running, 'rollback_product_images': products,
                'created_at': datetime.datetime.now(datetime.timezone.utc).isoformat()}
    atomic_json(destination / 'BACKUP.json', metadata)
    atomic_json(destination / 'SHA256.json', {p.name: sha(p) for p in destination.iterdir() if p.is_file() and p.name != 'INCOMPLETE.json'})
    (destination / 'INCOMPLETE.json').unlink()
    for p in destination.iterdir(): p.chmod(0o600)
    print('Complete cold backup (contains secrets): ' + str(destination))
    print('Services remain stopped; no automatic restart after backup.')
    return metadata


def backup_destination(dep, path):
    path = path.expanduser().resolve()
    if path.is_relative_to(dep.STATE) and not path.is_relative_to(dep.STATE / 'backup'):
        raise ValueError('Use a backup directory outside state or below state/backup')
    return path


def refresh_policies(dep, override):
    services = override.get('services', {})
    enabled = str(services.get('gateway', {}).get('environment', {}).get('MINDCREEK_GRAPH_ENABLED', 'false')).lower() == 'true'
    graph = dep.STATE / 'graph'
    if any(mount_parts(m)[1] in POLICIES for n in ('gateway', 'installer') for m in services.get(n, {}).get('volumes', [])):
        if graph.is_symlink(): raise ValueError('Graph directory must not be symlinked')
        graph.mkdir(mode=0o711, exist_ok=True)
        caps = json.loads((dep.ROOT / 'config/r3-capabilities.json').read_text())
        caps['capabilities']['rag_graph'] = enabled
        atomic_json(graph / 'capabilities.json', caps)
        atomic_json(graph / 'routes.json', json.loads((dep.ROOT / 'config/r3-routes.json').read_text()))
        for file in ('capabilities.json', 'routes.json'): (graph / file).chmod(0o644)


def start(dep, services):
    dep.validate(); dep.verify_images()
    names = list(dict.fromkeys(dep.CORE + (['neo4j'] if 'neo4j' in services else [])))
    dep.up_services(names)
    if json.loads(dep.install_command(['status'], True))['stage'] != 'ready':
        raise ValueError('Installation must already be ready; upgrade never creates admin or default space')
    dep.up_services(list(dict.fromkeys(names + ['frontend'] + [n for n in services if n not in ('ops', 'installer')])))


def upgrade(dep, old_package, destination):
    old, updated, report = plan(old_package, dep)
    dep.verify_images()
    # Validate the proposed override in an isolated state copy before stopping services.
    with tempfile.TemporaryDirectory(prefix='mindcreek-upgrade-config-') as tmp:
        copy = Path(tmp) / 'state'
        shutil.copytree(dep.STATE, copy, ignore=shutil.ignore_patterns('backup', '.release.lock'))
        atomic_json(copy / 'compose.override.json', updated)
        actual = dep.STATE
        try:
            dep.STATE = copy
            delegation_configuration(dep, apply=True)
            dep.validate()
        finally: dep.STATE = actual
    metadata = backup(old, destination)
    atomic_json(dep.STATE / 'upgrade-pending.json', {**report, 'backup': str(destination)})
    delegation_configuration(dep, apply=True)
    atomic_json(dep.STATE / 'compose.override.json', updated)
    refresh_policies(dep, updated)
    start(dep, metadata['running_services'])
    status = 'started_requires_access_review' if report['access_review_required'] else 'started'
    atomic_json(dep.STATE / 'active-release.json', {**report, 'backup': str(destination), 'status': status})
    (dep.STATE / 'upgrade-pending.json').unlink()
    print('Upgrade started successfully. Complete knowledge/API-key access review and browser/OAuth/model acceptance before reopening traffic.')


def restore(dep, source):
    if (source / 'INCOMPLETE.json').exists(): raise ValueError('Backup is incomplete')
    checks = json.loads((source / 'SHA256.json').read_text())
    for name, digest in checks.items():
        if Path(name).name != name or (source / name).is_symlink() or sha(source / name) != digest:
            raise ValueError('Backup checksum mismatch')
    data = json.loads((source / 'BACKUP.json').read_text())
    if data['release']['release'] != dep.release()['release'] or data['release']['images'] != dep.release()['images']:
        raise ValueError('Restore with the exact source release package and its loaded images')
    if dep.PROJECT == data['project'] or dep.STATE == Path(data['state']):
        raise ValueError('Restore requires a NEW project and NEW state directory; never overwrite live data')
    if containers(dep.PROJECT) or (dep.STATE.exists() and any(dep.STATE.iterdir())):
        raise ValueError('Restore target is not empty')
    existing = subprocess.check_output(['docker', 'volume', 'ls', '-q', '--filter', 'label=com.docker.compose.project=' + dep.PROJECT], text=True).strip()
    if existing: raise ValueError('Restore project already owns volumes')
    rollback_images = source/'rollback-product-images.tar'
    if rollback_images.is_file():
        subprocess.run(['docker', 'load', '--input', str(rollback_images)], check=True)
    dep.verify_images(); dep.STATE.mkdir(mode=0o700, parents=True, exist_ok=True)
    with tarfile.open(source / 'instance.tar.gz') as tar:
        for item in tar:
            parts = Path(item.name).parts
            if not parts or parts[0] != 'instance' or '..' in parts or item.issym() or item.islnk(): raise ValueError('Unsafe instance backup')
            target = dep.STATE.joinpath(*parts[1:])
            if item.isdir(): target.mkdir(parents=True, exist_ok=True)
            elif item.isfile():
                target.parent.mkdir(parents=True, exist_ok=True)
                with tar.extractfile(item) as src, target.open('wb') as dest: shutil.copyfileobj(src, dest)
            else: raise ValueError('Unsupported backup entry')
            target.chmod(item.mode & 0o777)
    override = dep.STATE / 'compose.override.json'
    if override.exists():
        # Only replace the known old instance path, never general model/OAuth settings.
        text = override.read_text().replace(str(Path(data['state'])) + '/', str(dep.STATE) + '/')
        atomic_json(override, json.loads(text))
    dep.read_env(); config = resolved(dep)
    target_volumes = {}
    for logical in data['volumes']:
        spec = config['volumes'][logical]
        name = spec.get('name', dep.PROJECT + '_' + logical)
        probe = subprocess.run(['docker', 'volume', 'inspect', name], capture_output=True)
        if spec.get('external') or probe.returncode == 0: raise ValueError('Restore volume already exists or is external: ' + logical)
        target_volumes[logical] = name
    for logical, name in target_volumes.items():
        subprocess.run(['docker', 'volume', 'create', '--label', 'com.docker.compose.project=' + dep.PROJECT,
                        '--label', 'com.docker.compose.volume=' + logical, name], check=True, stdout=subprocess.DEVNULL)
    volume_io(dep, source, target_volumes, 'restore')
    atomic_json(dep.STATE / 'restore-complete.json', {'from_project': data['project'], 'project': dep.PROJECT, 'release': dep.release()['release']})
    print('Restored into a new isolated project; services are NOT started. Review ports, UID/GID and domain, then validate/up.')


def graph(dep, enabled):
    support = module(dep.ROOT / 'bin/graph_support.py', 'graph_support')
    directory = dep.STATE / 'graph'
    if directory.is_symlink(): raise ValueError('Refusing graph symlink')
    directory.mkdir(mode=0o711, exist_ok=True)
    secret = directory / 'password'
    target = dep.STATE / 'compose.override.json'
    current = json.loads(target.read_text()) if target.exists() else {}
    services = current.setdefault('services', {})
    if not secret.exists():
        if 'neo4j' in services or not enabled: raise ValueError('Existing graph password is required; never reset a populated database')
        with open(secret, 'x', opener=lambda p, f: os.open(p, f, 0o600)) as stream: stream.write(secrets.token_hex(24) + '\n')
    dep.private(secret); password = secret.read_text().strip()
    neo = services.setdefault('neo4j', support.graph_service(password))
    if neo.get('image') != GRAPH_IMAGE or neo.get('environment', {}).get('NEO4J_AUTH') != 'neo4j/' + password:
        raise ValueError('Custom graph service/credentials require review')
    current.setdefault('volumes', {}).setdefault('mindcreek_graph_data', {})
    app = services.setdefault('app', {}).setdefault('environment', {})
    if enabled:
        services['app'].setdefault('depends_on', {})['neo4j'] = {'condition': 'service_healthy'}
    app.update(NEO4J_ENABLE=str(enabled).lower(), NEO4J_URI='bolt://neo4j:7687', NEO4J_USERNAME='neo4j', NEO4J_PASSWORD=password)
    for name in ('gateway', 'installer'):
        service = services.setdefault(name, {}); env = service.setdefault('environment', {})
        env['MINDCREEK_GRAPH_ENABLED'] = str(enabled).lower()
        if name == 'gateway':
            env.update(MINDCREEK_GRAPH_VIEW_URI='bolt://neo4j:7687' if enabled else '', MINDCREEK_GRAPH_VIEW_USERNAME='neo4j', MINDCREEK_GRAPH_VIEW_PASSWORD=password, MINDCREEK_GRAPH_VIEW_DATABASE='neo4j')
        mounts = service.setdefault('volumes', [])
        for dest, file in POLICIES.items():
            item = str(directory / file) + ':' + dest + ':ro'
            for mount in mounts:
                if mount_parts(mount)[1] == dest and mount != item: raise ValueError('Custom policy mount requires review')
            if item not in mounts: mounts.append(item)
    refresh_policies(dep, current)
    if target.exists(): shutil.copy2(target, dep.STATE / ('compose.override.before-graph-' + secrets.token_hex(4) + '.json'))
    atomic_json(target, current)
    print('Graph configuration prepared; existing data preserved. Run up after validating.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=Path, default=ROOT, help='Target package (source package when restoring)')
    subs = parser.add_subparsers(dest='op', required=True)
    for op in ('plan', 'upgrade'):
        p = subs.add_parser(op); p.add_argument('--from-package', type=Path, required=True)
        if op == 'upgrade': p.add_argument('--backup', type=Path, required=True)
    p = subs.add_parser('backup'); p.add_argument('--output', type=Path, required=True)
    p = subs.add_parser('restore'); p.add_argument('--backup', type=Path, required=True)
    p = subs.add_parser('graph'); p.add_argument('mode', choices=('enable', 'disable'))
    args = parser.parse_args(); state, _ = selection(); dep = deployment(args.package.resolve())
    if args.op == 'restore': restore(dep, args.backup.resolve()); return
    dep.read_env()
    with lock(state):
        if args.op == 'plan': print(json.dumps(plan(args.from_package.resolve(), dep)[2], ensure_ascii=False, indent=2))
        elif args.op == 'upgrade': upgrade(dep, args.from_package.resolve(), backup_destination(dep, args.backup))
        elif args.op == 'backup': backup(dep, backup_destination(dep, args.output))
        else: graph(dep, args.mode == 'enable')


if __name__ == '__main__':
    try: main()
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as exc:
        print('Maintenance stopped: ' + (str(exc) if not isinstance(exc, subprocess.CalledProcessError) else 'Docker operation failed; inspect local container diagnostics'), file=sys.stderr)
        print('Existing volumes are preserved. A failed upgrade is not automatically rolled back; use the matching cold backup.', file=sys.stderr)
        sys.exit(1)
