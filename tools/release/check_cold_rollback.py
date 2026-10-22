#!/usr/bin/env python3
"""Restore one supported release's synthetic pre-upgrade cold backup."""
import datetime
import json
import os
from pathlib import Path
import secrets
from current_smoke import CURRENT_WORKDIR, SOURCE_RELEASES, database_facts, graph_query, load

run = Path(os.environ.get('MINDCREEK_RELEASE_WORKDIR', CURRENT_WORKDIR / 'upgrade-r4'))
smoke = json.loads((run / 'smoke.json').read_text())
assert smoke['status'] == 'passed_with_limitations' and smoke['disposable_project_removed'], 'Complete and clean up source smoke first'
assert smoke['project'] == (run / 'smoke-project').read_text().strip()
state = Path((run / 'smoke-state').read_text().strip())
backup = state / 'backup/before-upgrade'
source = Path((run / 'package-path').read_text().strip())
target = Path((CURRENT_WORKDIR / 'package-path').read_text().strip())
recovery = run / ('rollback-' + secrets.token_hex(4))
project = 'mindcreek-cold-rollback-' + secrets.token_hex(4)
os.environ.update(MINDCREEK_STATE_DIR=str(recovery), MINDCREEK_PROJECT=project)
ops = load(target / 'bin/release', 'rollback_tools')
dep = load(source / 'bin/mindcreek', 'rollback_source')
target_release = json.loads((target / 'RELEASE.json').read_text())
source_release = dep.release()['release']
assert source_release == smoke['upgrade_from'] in SOURCE_RELEASES
assert smoke['release'] == target_release['release']
record = {'status': 'failed', 'release': target_release['release'],
          'package_runtime_sha256': target_release['package_runtime_sha256'],
          'upgrade_from': source_release, 'source_release': source_release, 'project': project,
          'time': datetime.datetime.now(datetime.timezone.utc).isoformat()}
try:
    ops.restore(dep, backup)
    metadata = json.loads((backup / 'BACKUP.json').read_text())
    assert metadata['release']['release'] == source_release
    ops.start(dep, metadata['running_services'])
    status = json.loads(dep.install_command(['status'], True))
    assert status['stage'] == 'ready'
    expected = smoke['source_database_facts']
    actual = database_facts(dep)
    assert actual == expected, (actual, expected)
    assert actual['migration_version'] == SOURCE_RELEASES[source_release]
    graph = graph_query(dep, 'MATCH (n:CURRENT_UPGRADE_MARKER) RETURN n.value')
    assert 'preserved-synthetic-graph' in graph
    record.update(status='passed', installation='ready',
                  source_product_migration_version=expected['migration_version'],
                  restored_product_migration_version=actual['migration_version'],
                  source_database_facts=expected, restored_database_facts=actual,
                  original_graph_marker=True,
                  old_product_addon_images_restored=bool(metadata['rollback_product_images']))
finally:
    if (recovery / 'enterprise.env').exists():
        dep.compose(['--profile', 'tls', '--profile', 'tools', 'down', '--volumes', '--remove-orphans'])
        record['disposable_project_removed'] = True
    (run / 'cold-rollback.json').write_text(json.dumps(record, indent=2) + '\n')
    print('Cold rollback verification: ' + record['status'])
