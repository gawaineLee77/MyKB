#!/usr/bin/env python3
"""Validate the synchronous graph-marker evidence from one completed upgrade smoke.

Seeding and post-upgrade reads happen inside current_smoke.additional_checks,
before backup and after startup respectively. This verifier never races a run.
"""
import json
import os
from pathlib import Path
from current_smoke import CURRENT_WORKDIR, SOURCE_RELEASES

run = Path(os.environ.get('MINDCREEK_RELEASE_WORKDIR', CURRENT_WORKDIR / 'upgrade-r4'))
report = json.loads((run / 'smoke.json').read_text())
marker = json.loads((run / 'upgrade-marker.json').read_text())
target = Path((CURRENT_WORKDIR / 'package-path').read_text().strip())
release = json.loads((target / 'RELEASE.json').read_text())
assert report['status'] == 'passed_with_limitations' and report['disposable_project_removed']
assert marker['status'] == 'passed'
assert marker['project'] == report['project']
assert marker['release'] == report['release'] == release['release']
assert marker['upgrade_from'] == report['upgrade_from'] in SOURCE_RELEASES
assert marker['source_release'] == marker['upgrade_from']
assert marker['package_runtime_sha256'] == report['package_runtime_sha256'] == release['package_runtime_sha256']
assert marker['old_graph_seeded_before_backup'] and marker['graph_preserved_after_upgrade']
assert marker['source_database_facts'] == report['source_database_facts']
print('PASS synchronous pre-upgrade Neo4j marker preserved: ' + marker['upgrade_from'])
