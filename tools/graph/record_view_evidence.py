"""Record completed, local graph-browser checks without private runtime data.

Run from the repository root after the commands in the acceptance record.
This records verification outputs; it does not itself run tests.
"""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
RUN = ROOT / '.local/graph-view'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    logs = {name: (RUN / name).read_text() for name in (
        'gateway-tests.log', 'gateway-race.log', 'frontend-tests.log',
        'typecheck.log', 'build.log', 'engineering.log', 'graph-config.log')}
    for name in ('gateway-tests.log', 'gateway-race.log'):
        assert 'internal/graphview' in logs[name] and not re.search(r'^FAIL', logs[name], re.M), name
    assert 'ℹ fail 0' in logs['frontend-tests.log']
    assert 'error TS' not in logs['typecheck.log'] and 'vue-tsc --build' in logs['typecheck.log']
    assert '✓ built in' in logs['build.log']
    assert 'Design documents verified' in logs['engineering.log'] and 'R4 native UI boundary' in logs['engineering.log']
    assert 'OK' in logs['graph-config.log']
    browser = json.loads((RUN / 'browser.json').read_text())
    assert browser['status'] == 'passed' and browser['node'].startswith('v24.')
    # Product source/config snapshot, excluding private environments and outputs.
    inputs = sorted([*ROOT.glob('services/gateway/**/*.go'),
                     *[p for p in (ROOT / 'tools/frontend-overlay').rglob('*') if p.is_file()],
                     ROOT / 'services/gateway/go.mod', ROOT / 'services/gateway/go.sum',
                     ROOT / 'deploy/enterprise/compose.json', ROOT / 'deploy/enterprise/enterprise.env.example',
                     ROOT / 'tools/graph/view_browser.mjs'])
    source = hashlib.sha256()
    for path in inputs:
        source.update(str(path.relative_to(ROOT)).encode() + b'\0' + path.read_bytes())
    result = {
        'status': 'passed', 'date': '2026-10-08',
        'scope': 'Local synthetic verification; compiled Vue + mocked HTTP and separate real disposable Neo4j tests. No enterprise OAuth/model/deployment acceptance.',
        'source_sha256': source.hexdigest(),
        'base_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        'upstream_commit': subprocess.check_output(['git', '-C', 'upstream/weknora', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        'upstream_clean': not subprocess.check_output(['git', '-C', 'upstream/weknora', 'status', '--porcelain'], cwd=ROOT, text=True).strip(),
        'frontend_tests': int(re.search(r'ℹ tests (\d+)', logs['frontend-tests.log'])[1]),
        'node': browser['node'], 'go': subprocess.check_output(['go', 'version'], text=True).strip(),
        'neo4j': '2025.10.1 / neo4j-go-driver v6.0.0',
        'gateway_uncached_output': logs['gateway-tests.log'].splitlines(),
        'gateway_race_output': logs['gateway-race.log'].splitlines(),
        'log_sha256': {name: digest(RUN / name) for name in logs},
        'browser': browser,
        'screenshots_sha256': {path: digest(ROOT / path) for path in browser['screenshots']},
        'pending': ['Enterprise target deployment and real data quality/performance', 'New AMD64 offline image package'],
    }
    target = ROOT / 'docs/plans/evidence/graph-view-verification.json'
    target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print('Recorded graph browser verification: ' + str(target.relative_to(ROOT)))


if __name__ == '__main__':
    main()
