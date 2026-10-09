#!/usr/bin/env python3
"""Build the current nine-image AMD64 release without rewriting R4/R5 evidence."""
import hashlib
import json
import subprocess
from pathlib import Path
import build_enterprise as builder
from r5_evidence import ui_digest

ROOT = Path(__file__).resolve().parents[2]
LOCAL = ROOT / '.local/enterprise-current'
ui = LOCAL / 'ui'
for name, needle in [('frontend-tests.log', 'fail 0'), ('build.log', 'built in'), ('typecheck.log', 'vue-tsc')]:
    if needle not in (LOCAL / name).read_text(): raise RuntimeError('Missing current validation: ' + name)
log = (LOCAL / 'gateway-tests.log').read_text()
if 'FAIL' in log or 'internal/server' not in log: raise RuntimeError('Gateway regression is not complete')
subprocess.run(['node', str(ROOT/'tools/frontend-overlay/check-native.mjs'), str(ui), str(ROOT/'upstream/weknora/frontend')], check=True)
h = hashlib.sha256()
for p in sorted((ui / 'dist').rglob('*')):
    if p.is_file(): h.update(str(p.relative_to(ui/'dist')).encode()+b'\0'+p.read_bytes())
proof = {'status':'passed', 'ui_source_sha256':ui_digest(), 'bundle_sha256':h.hexdigest()}
path = LOCAL / 'ui-build.json'; path.write_text(json.dumps(proof, indent=2)+'\n')
builder.LOCAL = LOCAL
builder.EXTERNAL = [*builder.EXTERNAL, 'mindcreek-neo4j:2025.10.1-graph1']
builder.build('current', path, ui)
