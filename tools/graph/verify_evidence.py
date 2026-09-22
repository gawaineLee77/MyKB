#!/usr/bin/env python3
"""Verify the graph extension's independent evidence without rewriting R2–R4 history."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools/redesign'))
from r2_evidence import source_digest


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    evidence = ROOT / 'docs/plans/evidence'
    release = json.loads((evidence / 'graph-release.json').read_text())
    api = json.loads((evidence / 'graph-api-probe.json').read_text())
    browser = json.loads((evidence / 'graph-browser.json').read_text())
    checks = json.loads((evidence / 'graph-checks.json').read_text())
    apply = json.loads((evidence / 'graph-apply.json').read_text())
    for value in [release, api, browser, checks, apply]:
        assert value['status'] == 'passed'
    assert api['gateway_source_sha256'] == checks['gateway_source_sha256'] == source_digest()
    assert release['graph_helper_sha256'] == apply['graph_helper_sha256'] == sha(ROOT / 'deploy/graph/graph.py')
    assert release['routes_sha256'] == sha(ROOT / 'config/r3-routes.json')
    assert api['probe_sha256'] == sha(ROOT / 'tools/redesign/r2_probe.py')
    assert browser['probe_sha256'] == sha(ROOT / 'tools/graph/graph_browser.mjs')
    assert api['cleanup'] == 'only disposable project removed'
    assert all(c['passed'] for c in api['checks'])
    assert all(r['actual_status'] in r['expected_status'] for r in api['requests'])
    required = {'graph.native_extraction_preview', 'graph.native_persisted_nodes', 'graph.native_persisted_edge',
        'graph.mcp_answer_has_graph_only_source', 'graph.native_agent_answer_with_sources',
        'graph.cross_space_denied_before_model', 'graph.machine_key_foreign_denied',
        'graph.restart_preserves_data', 'graph.reparse_idempotent', 'graph.delete_cleans_only_target_document',
        'employee.removed_not_readded', 'admin.password_rotation_revokes_access_and_refresh'}
    assert required <= {c['name'] for c in api['checks']}
    assert {'native_toggle_preview_save_reload', 'viewer_graph_agent_stream'} <= set(browser['checks'])
    assert browser['graph_stream_source_verified']
    assert release['screenshots_inspected'] and len(browser['screenshots']) == 4
    assert all((ROOT / path).is_file() for path in browser['screenshots'])
    assert all(c['exit_code'] == 0 for c in checks['checks'])
    for path, digest in release['verified_files'].items():
        assert sha(ROOT / path) == digest, 'Changed after verification: ' + path
    assert not subprocess.check_output(['git', '-C', str(ROOT / 'upstream/weknora'), 'status', '--porcelain'], text=True).strip()
    print('PASS graph source/evidence, API scope/recovery, browser screenshots and clean upstream')


if __name__ == '__main__': main()
