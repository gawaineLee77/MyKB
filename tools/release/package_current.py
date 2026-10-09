#!/usr/bin/env python3
"""Package the current release while preserving all historical archives/evidence."""
import argparse
import json
import shutil
from pathlib import Path
import package_enterprise as package

package.LOCAL = package.ROOT / '.local/enterprise-current'
args = argparse.ArgumentParser(description=__doc__)
args.add_argument('command', choices=['assemble', 'export', 'finalize'])
a = args.parse_args()
if a.command == 'assemble':
    target = package.assemble()
    record = json.loads((target/'RELEASE.json').read_text())
    record.update(installation_modes=['fresh', 'upgrade-v0.8-enterprise', 'isolated-cold-restore'],
                  features=['R5 employee assistant', 'native knowledge graph browser', 'editable Word export'],
                  state_schema='R2-R5 workspace-centric', optional_images=['mindcreek-neo4j:2025.10.1-graph1'])
    (target/'RELEASE.json').write_text(json.dumps(record,indent=2)+'\n')
else:
    target = Path((package.LOCAL/'package-path').read_text().strip())
    if a.command == 'finalize':
        shutil.copy2(package.LOCAL/'upgrade-smoke/smoke.json', target/'evidence/upgrade-smoke.json')
        shutil.copy2(package.LOCAL/'installer-python312.log', target/'evidence/installer-python312.log')
        shutil.copy2(package.LOCAL/'ui-build.json', target/'evidence/ui-build.json')
        for name in ('upgrade-marker.json','cold-rollback.json'):
            shutil.copy2(package.LOCAL/name, target/'evidence'/name)
    getattr(package, a.command)(target)
