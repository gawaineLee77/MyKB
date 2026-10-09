#!/usr/bin/env python3
"""Recover the pre-upgrade R4 cold backup into a new project, proving rollback."""
import datetime
import json
import os
from pathlib import Path
import secrets
import time
from current_smoke import load
ROOT=Path(__file__).resolve().parents[2];LOCAL=ROOT/'.local/enterprise-current';run=LOCAL/'upgrade-smoke'
state=Path((run/'smoke-state').read_text().strip());backup=state/'backup/before-upgrade'
source=Path((run/'package-path').read_text().strip());target=Path((LOCAL/'package-path').read_text().strip())
recovery=LOCAL/('rollback-'+secrets.token_hex(4));project='mindcreek-cold-rollback-'+secrets.token_hex(4)
os.environ.update(MINDCREEK_STATE_DIR=str(recovery),MINDCREEK_PROJECT=project)
ops=load(target/'bin/release','rollback_tools');dep=load(source/'bin/mindcreek','rollback_source')
record={'status':'failed','release':json.loads((target/'RELEASE.json').read_text())['release'],
        'source_release':dep.release()['release'],'project':project,'time':datetime.datetime.now(datetime.timezone.utc).isoformat()}
try:
    # Wait for the source test's completed backup and project cleanup. This avoids
    # binding the restored frontend to ports still used by the source project.
    deadline=time.monotonic()+1500
    while time.monotonic()<deadline:
        report=run/'smoke.json'
        if report.exists():
            value=json.loads(report.read_text())
            if value.get('project')==(run/'smoke-project').read_text().strip() and value.get('disposable_project_removed'):break
        time.sleep(3)
    else:raise RuntimeError('Source test has not completed')
    ops.restore(dep,backup)
    metadata=json.loads((backup/'BACKUP.json').read_text())
    ops.start(dep,metadata['running_services'])
    status=json.loads(dep.install_command(['status'],True))
    assert status['stage']=='ready'
    counts=dep.compose(['exec','-T','postgres','psql','-U','mindcreek','-d','mindcreek_r4','-Atc',
        "SELECT count(*) FROM users; SELECT count(*) FROM knowledge_bases; SELECT count(*) FROM knowledges; SELECT to_regclass('mindcreek.assistant_sessions') IS NULL"],True).strip().splitlines()
    assert counts==['2','2','3','t'],counts
    graph=dep.compose(['exec','-T','neo4j','sh','-c','NEO4J_USERNAME=neo4j NEO4J_PASSWORD="${NEO4J_AUTH#neo4j/}" cypher-shell -a bolt://localhost:7687 "$1"','sh',
        'MATCH (n:CURRENT_UPGRADE_MARKER) RETURN n.value'],True)
    assert 'preserved-synthetic-graph' in graph
    record.update(status='passed',installation='ready',users=2,knowledge_bases=2,documents=3,
        original_graph_marker=True,r5_schema_absent=True,old_product_addon_images_restored=True)
finally:
    if (recovery/'enterprise.env').exists():
        dep.compose(['--profile','tls','--profile','tools','down','--volumes','--remove-orphans'])
        record['disposable_project_removed']=True
    (LOCAL/'cold-rollback.json').write_text(json.dumps(record,indent=2)+'\n')
    print('Cold rollback verification: '+record['status'])
