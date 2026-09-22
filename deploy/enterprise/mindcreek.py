#!/usr/bin/env python3
"""Portable R4 deployment helper; requires Python 3.10+ and Docker Compose."""
from __future__ import annotations
import getpass
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
STATE = Path(os.environ.get('MINDCREEK_STATE_DIR', ROOT / 'instance')).expanduser().resolve()
PROJECT = os.environ.get('MINDCREEK_PROJECT', 'mindcreek-enterprise-r4')
CORE = ['postgres', 'redis', 'docreader', 'app', 'gateway']


def run(args, capture=False, env=None):
    return subprocess.run(args, check=True, text=True, stdout=subprocess.PIPE if capture else None,
                          env=env, cwd=ROOT).stdout


def private(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_mode & 0o077:
        raise ValueError(f'{path}: must be a regular private file (chmod 600)')


def read_env():
    path = STATE / 'enterprise.env'
    private(path)
    result = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith('#'): continue
        if '=' not in line: raise ValueError('Invalid env entry; expected KEY=value')
        k, v = line.split('=', 1); k = k.strip(); v = v.strip()
        if not re.fullmatch('[A-Z][A-Z0-9_]*', k) or k in result:
            raise ValueError('Invalid or duplicate env name: ' + k)
        if v.startswith(("'", '"')):
            if len(v) < 2 or v[-1] != v[0]: raise ValueError('Unclosed quote: ' + k)
            v = v[1:-1]
        result[k] = v
    return result


def init():
    if (STATE / 'enterprise.env').exists() or (STATE / 'secrets').exists():
        raise ValueError('Instance already exists; init never overwrites it')
    password = getpass.getpass('Initial local admin password (12-32 characters, letters and digits): ')
    validate_password(password)
    if password != getpass.getpass('Repeat password: '):
        raise ValueError('Password mismatch')
    initialize(password)
    print(f'Created {STATE}; edit enterprise.env, then run validate. Password was not printed.')


def initialize(password):
    """Also used by isolated synthetic acceptance, never with company credentials."""
    validate_password(password)
    if (STATE / 'enterprise.env').exists() or (STATE / 'secrets').exists():
        raise ValueError('Instance already exists')
    STATE.mkdir(mode=0o700, parents=True, exist_ok=True); STATE.chmod(0o700)
    for n in ['secrets', 'runtime', 'ca', 'tls', 'backup']:
        (STATE / n).mkdir(mode=0o700)
    values = {'UID':str(os.getuid()), 'GID':str(os.getgid()), 'SYSTEM_AES_KEY':secrets.token_hex(16)}
    for n in ['DB_PASSWORD','JWT_SECRET','REDIS_PASSWORD','BROKER_SECRET']: values[n] = secrets.token_hex(32)
    content = (ROOT / 'enterprise.env.example').read_text()
    for n, v in values.items(): content = content.replace('__' + n + '__', v)
    for path, data in [(STATE/'enterprise.env', content), (STATE/'secrets/admin-password', password+'\n')]:
        with open(path, 'x', opener=lambda p, flags: os.open(p, flags, 0o600)) as f: f.write(data)


def validate_password(password):
    if not 12 <= len(password) <= 32 or not re.search('[A-Za-z]',password) or not re.search('[0-9]',password) or password != password.strip() or any(c in password for c in '\r\n\t'):
        raise ValueError('Admin password requires 12-32 characters, letters and digits, without whitespace padding or control characters')


def release():
    return json.loads((ROOT / 'RELEASE.json').read_text())


def docker_env(values=None, extra=None):
    values = read_env() if values is None else values
    # File values win over the caller's shell; bootstrap is transient and explicit.
    env = dict(os.environ)
    # Do not silently inherit application settings from the operator's shell.
    for name in re.findall(r'\$\{([A-Z][A-Z0-9_]*)', (ROOT/'compose.json').read_text()):
        env.pop(name, None)
    env.update({**values, 'MINDCREEK_STATE_DIR':str(STATE), 'MINDCREEK_RELEASE':release()['release']})
    env['MINDCREEK_INSTALL_REGISTRATION_DISABLED'] = 'true'
    env['MINDCREEK_INSTALL_BOOTSTRAP_EMAIL'] = ''
    env.update(extra or {})
    return env


def compose(args, capture=False, extra=None):
    cmd = ['docker','compose','--project-name',PROJECT,'--project-directory',str(ROOT),
           '--env-file','/dev/null','-f',str(ROOT/'compose.json')]
    # Operator-owned local additions (e.g. extra CA-aware network configuration).
    override = STATE/'compose.override.json'
    if override.exists(): cmd += ['-f',str(override)]
    return run(cmd + args, capture=capture, env=docker_env(extra=extra))


def verify_images():
    for wanted in release()['images']:
        got = json.loads(run(['docker','image','inspect','--platform','linux/amd64',wanted['name']],True))[0]
        # Containerd reports a platform manifest ID; classic Docker reports the config ID.
        if got['Id'] not in {wanted['id'],wanted.get('config_digest')} or (got['Os'],got['Architecture']) != ('linux','amd64'):
            raise ValueError('Image differs from the package lock: ' + wanted['name'])
    print('All 8 runtime images match the AMD64 lock.')


def validate():
    values = read_env()
    for k,v in values.items():
        if 'REPLACE' in v or 'example.invalid' in v or v.startswith('__'):
            raise ValueError('Fill deployment setting: ' + k)
    for k in ['DB_PASSWORD','JWT_SECRET','REDIS_PASSWORD','MINDCREEK_BROKER_CLIENT_SECRET']:
        if len(values.get(k,'')) < 24: raise ValueError(k + ': at least 24 characters required')
    if not re.fullmatch(r'[A-Za-z0-9_-]+',values['DB_PASSWORD']):
        raise ValueError('DB_PASSWORD must be URL-safe letters/digits/underscore/hyphen for the database DSN')
    for k in ['DB_USER','DB_NAME']:
        if not re.fullmatch(r'[a-z][a-z0-9_]*',values[k]): raise ValueError('Invalid ' + k)
    if values.get('MINDCREEK_DEPLOYMENT_ENV') not in ['staging','production']:
        raise ValueError('Enterprise bundle requires staging or production')
    origin = urlsplit(values.get('MINDCREEK_EXTERNAL_ORIGIN',''))
    if origin.scheme != 'https' or not origin.hostname or origin.path not in ['', '/'] or origin.query or origin.fragment or origin.username:
        raise ValueError('MINDCREEK_EXTERNAL_ORIGIN must be an HTTPS origin without a path')
    if '@' not in values.get('MINDCREEK_INSTALL_ADMIN_EMAIL',''):
        raise ValueError('Installation admin email required')
    for k in ['MINDCREEK_UID','MINDCREEK_GID']:
        if not values.get(k,'').isdigit(): raise ValueError('Invalid ' + k)
    private(STATE/'secrets/admin-password')
    if int(values['MINDCREEK_UID']) != (STATE/'secrets/admin-password').stat().st_uid:
        raise ValueError('MINDCREEK_UID must own the secret directory and its files')
    validate_password((STATE/'secrets/admin-password').read_text().rstrip('\n'))
    whitelist = [v.strip() for v in values.get('SSRF_WHITELIST_EXTRA','').split(',') if v.strip()]
    if any('*' in h or '/' in h or ':' in h for h in whitelist):
        raise ValueError('SSRF_WHITELIST_EXTRA accepts exact hostnames or IPv4 addresses only')
    if 'gateway' in whitelist: whitelist.remove('gateway')
    for prefix in ['LLM','EMBEDDING','RERANK'] + (['VLM'] if values.get('MINDCREEK_MANAGED_VLM_ENABLED')=='true' else []):
        host = urlsplit(values.get(f'MINDCREEK_MANAGED_{prefix}_BASE_URL','')).hostname
        if host not in whitelist: raise ValueError(f'Add the approved {prefix} host to SSRF_WHITELIST_EXTRA')
    script = ROOT/'scripts/render-phase5-models.py'
    run([sys.executable,str(script),'--env-file',str(STATE/'enterprise.env'),'--output',str(STATE/'runtime/builtin_models.yaml')],env=docker_env(values))
    # YAML has only variable references, so the app can safely read it as another UID.
    (STATE/'runtime/builtin_models.yaml').chmod(0o644)
    ca = (ROOT/'config/public-ca.crt').read_bytes()
    for cert in sorted((STATE/'ca').glob('*.crt')):
        data = cert.read_bytes()
        if b'PRIVATE KEY' in data or b'BEGIN CERTIFICATE' not in data:
            raise ValueError('CA directory accepts PEM certificates only: ' + cert.name)
        ca += b'\n' + data
    (STATE/'runtime/ca-bundle.pem').write_bytes(ca)
    (STATE/'runtime/ca-bundle.pem').chmod(0o644)
    compose(['config','--quiet'])
    print('Configuration validated; no credentials printed.')


def up_services(names, extra=None, recreate=False):
    args = ['up','-d','--pull','never','--no-build','--wait','--wait-timeout','600']
    if recreate: args += ['--no-deps','--force-recreate']
    compose(args+names, extra=extra)


def install_command(args, capture=False):
    # Dedicated service: Compose v5 preserves read_only on a run -v override.
    # Normal gateway access stays read-only; only this one-shot installer can write.
    return compose(['run','--rm','--no-deps','--pull','never','installer','install',*args],capture)


def install(args):
    validate(); verify_images()
    op = args[0] if args else 'status'
    if op != 'admin':
        if op not in ['status','default-space','repair-member-key']: raise ValueError('Unknown install operation')
        install_command(args or ['status']); return
    # Never expose the site while temporarily permitting the private admin creation.
    compose(['stop','frontend','tls'])
    up_services(CORE)
    status = json.loads(install_command(['status'],True))
    if status['stage'] in ['account_ready','space_creating','space_ready','key_creating','ready']:
        print(json.dumps(status,ensure_ascii=False)); return
    try:
        if status['stage'] in ['new','registering']:
            up_services(['app'],{'MINDCREEK_INSTALL_REGISTRATION_DISABLED':'false'},True)
            status = json.loads(install_command(['prepare',*args[1:]],True))
        up_services(['app'],{'MINDCREEK_INSTALL_BOOTSTRAP_EMAIL':status['admin_email']},True)
        install_command(['confirm-admin'])
    except subprocess.CalledProcessError:
        # Preserve this container's diagnostics before the closing-window recreate.
        path = STATE/'installation-error.log'
        with open(path, 'w', opener=lambda p, flags: os.open(p, flags, 0o600)) as f:
            f.write(compose(['logs','--tail','150','app'],capture=True))
        path.chmod(0o600)
        print('Private installation diagnostics: '+str(path),file=sys.stderr)
        raise
    finally:
        up_services(['app'],recreate=True)


def verify_files():
    for line in (ROOT/'SHA256SUMS').read_text().splitlines():
        expected, relative = line.split('  ',1)
        path = ROOT/relative
        if not path.resolve().is_relative_to(ROOT) or path.is_symlink(): raise ValueError('Invalid checksum path')
        h = hashlib.sha256()
        with path.open('rb') as f:
            for block in iter(lambda:f.read(8*1024*1024),b''): h.update(block)
        if h.hexdigest() != expected: raise ValueError('Checksum mismatch: ' + relative)
    print('Package checksums verified.')


def main():
    args = sys.argv[1:]; op = args.pop(0) if args else 'help'
    if op == 'init': init()
    elif op == 'verify': verify_files()
    elif op == 'load':
        verify_files(); run(['docker','load','--input',str(ROOT/'images.tar')]); verify_images()
    elif op == 'images': verify_images()
    elif op == 'validate': validate()
    elif op == 'install': install(args)
    elif op == 'up':
        validate(); verify_images()
        if json.loads(install_command(['status'],True))['stage'] != 'ready':
            raise ValueError('Finish install admin and install default-space before up')
        if args not in [[],['--tls']]: raise ValueError('Usage: up [--tls]')
        if args:
            for n in ['fullchain.pem','privkey.pem']:
                if not (STATE/'tls'/n).is_file(): raise ValueError('Missing TLS file: '+n)
            private(STATE/'tls/privkey.pem')
        up_services(CORE+['frontend']+(['tls'] if args else []))
    elif op == 'stop': compose(['stop'])
    elif op == 'ps': compose(['ps','-a'])
    elif op == 'logs': compose(['logs','--tail','100',*args])
    elif op == 'compose': compose(args)
    else:
        print('Usage: bin/mindcreek {init|verify|load|images|validate|install [admin|status|default-space|repair-member-key]|up [--tls]|ps|logs [service]|stop|compose ...}')
        print('State: '+str(STATE)+'; project: '+PROJECT)


if __name__ == '__main__':
    try: main()
    except (ValueError,OSError,KeyError,subprocess.CalledProcessError) as exc:
        # Command arguments contain paths and operation names only, never secrets.
        print('Error: '+str(exc),file=sys.stderr); sys.exit(1)
