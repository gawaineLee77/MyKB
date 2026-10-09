#!/usr/bin/env python3
"""Record completed local Word-export checks; never substitutes for executing them."""
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
BUILD = ROOT / '.local/word-export'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    core = json.loads((BUILD / 'browser-core.json').read_text())
    ui = json.loads((BUILD / 'browser-ui.json').read_text())
    assert core['status'] == ui['status'] == 'passed'
    assert 'pass 608' in (BUILD / 'frontend-tests.log').read_text()
    assert 'fail 0' in (BUILD / 'frontend-tests.log').read_text()
    assert 'error TS' not in (BUILD / 'typecheck.log').read_text()
    assert 'built in' in (BUILD / 'build.log').read_text()
    assert '40 documents' in (BUILD / 'engineering.log').read_text()
    with zipfile.ZipFile(BUILD / 'editable-answer.docx') as archive:
        doc = ET.fromstring(archive.read('word/document.xml'))
        ns = {'m': 'http://schemas.openxmlformats.org/officeDocument/2006/math', 'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
        assert len(doc.findall('.//m:oMath', ns)) == 11
        assert len(doc.findall('.//w:tc', ns)) == 12
    paths = [*sorted((ROOT / 'tools/frontend-overlay/native').glob('word-*')),
             ROOT / 'tools/frontend-overlay/native/WordExportButton.vue',
             ROOT / 'tools/frontend-overlay/apply-word-export.mjs',
             ROOT / 'tools/frontend-overlay/apply.mjs',
             ROOT / 'tools/frontend-overlay/check-native.mjs',
             *sorted((ROOT / 'tools/word-export').glob('*.mjs')),
             ROOT / 'testdata/word-export/answer.md']
    upstream = subprocess.check_output(['git', '-C', str(ROOT / 'upstream/weknora'), 'status', '--porcelain'], text=True)
    assert not upstream
    report = {
        'date': '2026-10-09',
        'status': 'implementation_and_automated_checks_passed',
        'scope': 'Synthetic main-site single-answer DOCX export; no deployment or enterprise OAuth/model calls',
        'source_sha256': {str(p.relative_to(ROOT)): digest(p) for p in paths},
        'node': core['node'], 'frontend_tests': 608,
        'browser_core': core, 'browser_ui': ui,
        'upstream_clean': True,
        'sample': {'equations_omml': 11, 'table_cells': 12, 'sha256': digest(BUILD / 'editable-answer.docx')},
        'render': {'engine': 'Bundled LibreOffice', 'pages_inspected': 2, 'cjk_fonts': 'Local fontconfig override using system fonts; no font redistributed'},
        'word_desktop': {'open': 'passed', 'display': 'passed', 'edit_save': 'pending', 'reason': 'Installed Microsoft Word is not activated and explicitly shows View Only'},
        'logs_sha256': {name: digest(BUILD / name) for name in ['frontend-tests.log', 'typecheck.log', 'build.log', 'browser-core.json', 'browser-ui.json', 'engineering.log', 'render.log']},
        'screenshots_sha256': {p: digest(ROOT / p) for p in ui['screenshots']},
    }
    destination = ROOT / 'docs/plans/evidence/word-export-verification.json'
    destination.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print('Word export evidence recorded; native Word edit/save remains pending activation.')


if __name__ == '__main__':
    main()
