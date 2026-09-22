"""Exercise the unchanged native UI against the disposable graph stack."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import threading


def verify_browser(root, run, base, admin, viewer, tenant, kb, agent):
    sys.path.insert(0, str(root / 'tools/redesign'))
    from r4_ui_server import create_server
    from r4_evidence import ui_digest
    directory = Path((root / '.local/redesign-r4/ui-path').read_text().strip()) / 'dist'
    build = json.loads((root / 'docs/plans/evidence/r4-ui-build.json').read_text())
    assert build['ui_source_sha256'] == ui_digest(), 'Rebuild UI before browser verification'
    digest = hashlib.sha256()
    for p in sorted(directory.rglob('*')):
        if p.is_file(): digest.update(str(p.relative_to(directory)).encode() + b'\0' + p.read_bytes())
    assert build['bundle_sha256'] == digest.hexdigest()
    fixture = run / 'browser-private.json'
    fixture.write_text(json.dumps({'admin': admin, 'viewer': viewer, 'tenant': tenant, 'kb': kb, 'agent': agent}))
    fixture.chmod(0o600)
    server = create_server(directory, base)
    thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
    node = os.environ.get('GRAPH_NODE_BIN')
    if not node:
        node = str(next((root / '.local/npm-ci-tools/_npx').glob('*/node_modules/node/bin/node')))
    assert subprocess.check_output([node, '--version'], text=True).startswith('v24.')
    try:
        subprocess.run([node, str(root / 'tools/graph/graph_browser.mjs')], check=True, env={**os.environ,
            'GRAPH_ROOT': str(root), 'GRAPH_FIXTURE': str(fixture),
            'GRAPH_ORIGIN': 'http://127.0.0.1:' + str(server.server_port)})
    finally:
        server.shutdown(); server.server_close(); thread.join(); fixture.unlink(missing_ok=True)
