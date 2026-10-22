#!/usr/bin/env python3
"""Inspect historical images or apply an explicitly reviewed ownership manifest."""
import argparse
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
MAX_MANIFEST_BYTES = 4 * 1024 * 1024


def positive(value):
    result = int(value)
    if result <= 0:
        raise argparse.ArgumentTypeError('must be positive')
    return result


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    audit = commands.add_parser('audit', help='Read-only ownership report as JSON on stdout')
    audit.add_argument('--tenant', type=positive, required=True)
    audit.add_argument('--limit', type=positive, default=1000)
    audit.add_argument('--after', default='', help='Continue after the resource ID returned by the previous audit page')
    apply = commands.add_parser('apply', help='Apply only an operator-reviewed manifest')
    apply.add_argument('--tenant', type=positive, required=True)
    apply.add_argument('--confirm-reviewed', action='store_true', required=True)
    apply.add_argument('--manifest', type=Path, required=True)
    return parser.parse_args(argv)


def manifest_input(args):
    if args.command != 'apply':
        return None
    path = args.manifest
    if path.is_symlink() or not path.is_file() or path.stat().st_size > MAX_MANIFEST_BYTES:
        raise ValueError('Manifest must be a regular JSON file no larger than 4 MiB')
    raw = path.read_bytes()
    value = json.loads(raw)
    if not isinstance(value, dict) or type(value.get('tenant_id')) is not int or value['tenant_id'] != args.tenant:
        raise ValueError('Manifest tenant_id must match --tenant')
    return raw


def load_ops():
    path = ROOT / 'bin/release'
    loader = importlib.machinery.SourceFileLoader('image_resource_release', str(path))
    spec = importlib.util.spec_from_loader(loader.name, loader)
    result = importlib.util.module_from_spec(spec)
    loader.exec_module(result)
    return result


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)
    raw = manifest_input(args)
    ops = load_ops()
    state, project = ops.selection()
    dep = ops.deployment(ROOT)
    # Verify the selected project's secret mount belongs to this installation.
    if dep.STATE != state or dep.PROJECT != project:
        raise ValueError('Package and selected instance do not agree')
    ops.confirm_instance(dep, ops.resolved(dep))
    cmd = [sys.executable, str(ROOT / 'bin/mindcreek'), 'compose', 'exec', '-T', 'gateway',
           '/mindcreek-gateway', 'image-resources', args.command, '--tenant', str(args.tenant)]
    if args.command == 'audit':
        cmd.extend(['--limit', str(args.limit)])
        if args.after:
            cmd.extend(['--after', args.after])
    else:
        cmd.append('--confirm-reviewed')
    # Forward bytes through stdin; operator files are never mounted in containers.
    subprocess.run(cmd, input=raw, check=True, cwd=ROOT, env=dict(os.environ))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as exc:
        print('Error: ' + str(exc), file=sys.stderr)
        raise SystemExit(1)
