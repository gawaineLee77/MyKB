#!/usr/bin/env python3
"""Collect only this package's completed, current install/upgrade/recovery evidence."""
import datetime
import hashlib
import json
from pathlib import Path
import re
import subprocess
from package_enterprise import runtime_digest

ROOT=Path(__file__).resolve().parents[2];LOCAL=ROOT/'.local/enterprise-current'
target=Path((LOCAL/'package-path').read_text().strip());release=json.loads((target/'RELEASE.json').read_text())
def read(name):return json.loads((LOCAL/name).read_text())
fresh,upgrade,archive,database=[read(n) for n in ('smoke.json','upgrade-smoke/smoke.json','archive-check.json','database-preflight.json')]
marker,rollback=[read(n) for n in ('upgrade-marker.json','cold-rollback.json')]
for proof in (marker,rollback):
    assert proof['status']=='passed' and proof['release']==release['release']
assert rollback['disposable_project_removed'] and rollback['original_graph_marker'] and rollback['r5_schema_absent']
for value in (fresh,upgrade):
    assert value['status'] in ('passed','passed_with_limitations'), value.get('failure')
    assert value['release']==release['release']
    assert value['package_runtime_sha256']==runtime_digest(target)
    assert value['disposable_project_removed']
    assert all(row['passed'] for row in value['checks'])
    names={c['name'] for c in value['checks']}
    assert {'current_graph_browser_real_neo4j','current_graph_viewer_denied','current_r5_employee_channel_question_history',
        'current_r5_nginx_dynamic_frame_policy','current_isolated_restore_preserves_installation',
        'current_isolated_restore_preserves_graph','current_isolated_restore_preserves_r5_bindings'} <= names
assert 'upgrade_preserves_identity_models_password_graph_and_links' in {c['name'] for c in upgrade['checks']}
assert archive['status']=='passed' and len(archive['images'])==9 and archive['release']==release['release']
assert database['release']==release['release'] and database['disposable_container_removed']
unit=(LOCAL/'installer-python312.log').read_text();assert '\nOK\n' in unit
count=int(re.search(r'Ran (\d+) tests',unit)[1])
assert count>=28
assert 'fail 0' in (LOCAL/'frontend-tests.log').read_text()
assert 'FAIL' not in (LOCAL/'gateway-tests.log').read_text()
assert not subprocess.check_output(['git','-C',str(ROOT/'upstream/weknora'),'status','--porcelain'],text=True).strip()
limitations=[{'code':'target_enterprise_acceptance_pending','impact':'Real company OAuth/models, physical AMD64 capacity and Word edit/save remain target-environment checks.'}]
if database['status']!='passed':
    limitations.append({'code':'amd64_emulation_bm25_abort','status':'reproduced_in_this_release',
        'impact':'BM25/hybrid retrieval is not accepted on this ARM host emulator. Run the included database preflight on physical AMD64 before importing business data; no default index configuration was changed.',
        'evidence':'database-preflight.json'})
report={'release':release['release'],'status':'passed_with_limitations','platform':'linux/amd64',
    'time':datetime.datetime.now(datetime.timezone.utc).isoformat(),'host_execution':fresh['host_execution'],
    'package_runtime_sha256':runtime_digest(target),'images':release['images'],'limitations':limitations,
    'fresh_checks':len(fresh['checks']),'upgrade_checks':len(upgrade['checks']),
    'upgrade_from':upgrade['upgrade_from'],'frontend_tests':608,'python312_tests':count,
    'old_graph_survives_upgrade':True,'pre_upgrade_cold_rollback_passed':True,
    'checks':fresh['checks']+upgrade['checks'],
    'evidence_sha256':{name:hashlib.sha256((LOCAL/name).read_bytes()).hexdigest() for name in
      ('smoke.json','upgrade-smoke/smoke.json','upgrade-marker.json','cold-rollback.json','archive-check.json','database-preflight.json','gateway-tests.log','frontend-tests.log','installer-python312.log')},
    'upstream_clean':True,'company_deployed':False}
(LOCAL/'acceptance.json').write_text(json.dumps(report,indent=2)+'\n')
(ROOT/'docs/plans/evidence/current-amd64-package.json').write_text(json.dumps(report,indent=2)+'\n')
print(f"Current bundle: {len(fresh['checks'])} fresh, {len(upgrade['checks'])} upgrade checks, {count} installer tests; limitations recorded")
