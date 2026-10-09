#!/usr/bin/env python3
"""Independent Neo4j data-preservation probe during the disposable old-package upgrade."""
import datetime
import json
import os
from pathlib import Path
import subprocess
import time
from current_smoke import load
ROOT=Path(__file__).resolve().parents[2];LOCAL=ROOT/'.local/enterprise-current'
run=LOCAL/'upgrade-smoke';state=Path((run/'smoke-state').read_text().strip());project=(run/'smoke-project').read_text().strip()
os.environ.update(MINDCREEK_STATE_DIR=str(state),MINDCREEK_PROJECT=project)
old=load(Path((run/'package-path').read_text().strip())/'bin/mindcreek','marker_source')
new=load(Path((LOCAL/'package-path').read_text().strip())/'bin/mindcreek','marker_target')
name=project+'-neo4j-1';deadline=time.monotonic()+900
while time.monotonic()<deadline:
    r=subprocess.run(['docker','inspect',name,'--format','{{.State.Health.Status}}'],capture_output=True,text=True)
    if r.returncode==0 and r.stdout.strip()=='healthy':break
    time.sleep(2)
else:raise RuntimeError('Old graph not ready')
assert not (state/'upgrade-pending.json').exists() and not (state/'backup/before-upgrade').exists(), 'Seed must precede backup'
def query(dep,sql):
    return dep.compose(['exec','-T','neo4j','sh','-c','NEO4J_USERNAME=neo4j NEO4J_PASSWORD="${NEO4J_AUTH#neo4j/}" cypher-shell -a bolt://localhost:7687 "$1"','sh',sql],True)
query(old,"CREATE (:CURRENT_UPGRADE_MARKER {value:'preserved-synthetic-graph'})")
while time.monotonic()<deadline:
    if (state/'active-release.json').exists():break
    time.sleep(2)
else:raise RuntimeError('Upgrade not completed')
result=query(new,'MATCH (n:CURRENT_UPGRADE_MARKER) RETURN n.value')
assert 'preserved-synthetic-graph' in result
record={'status':'passed','release':new.release()['release'],'source_release':old.release()['release'],
        'project':project,'old_graph_seeded_before_backup':True,'graph_preserved_after_upgrade':True,
        'time':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(LOCAL/'upgrade-marker.json').write_text(json.dumps(record,indent=2)+'\n')
print('PASS old Neo4j data preserved after upgrade')
