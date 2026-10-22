#!/usr/bin/env python3
"""Collect source-bound fresh, historical upgrade and cold-recovery acceptance."""
import datetime
import json
import os
from pathlib import Path

from build_current import ROOT, sha, validated_proof, workdir
from package_enterprise import runtime_digest
from release_baselines import UPGRADE_BASELINES, SOURCE_RELEASES, source_has_knowledge_acl, upgrade_key_check
from image_resources_smoke import CHECKS as IMAGE_CHECKS

COMMON_CHECKS = IMAGE_CHECKS | {
    'current_graph_browser_real_neo4j', 'current_graph_viewer_denied',
    'current_r5_employee_channel_question_history', 'current_r5_nginx_dynamic_frame_policy',
    'current_isolated_restore_preserves_installation', 'current_isolated_restore_preserves_graph',
    'current_isolated_restore_preserves_permissions_icons_and_r5', 'current_isolated_restore_preserves_branding',
    'current_kb_default_creator_private', 'current_public_kb_viewer_content_without_original_file',
    'current_six_mcp_tools', 'current_wiki_mcp_search_and_read', 'current_wiki_mcp_private_and_empty_scope_denied',
    'current_global_branding_default', 'current_global_branding_local_admin_and_public_logo',
    'current_agent_icon_upload_reference_and_draft_isolation', 'current_workspace_admin_explicit_delegation',
    'current_kb_collaboration_explicit_delegation', 'current_admin_own_keys_owner_all_keys',
    'current_machine_key_scope_and_role_revocation',
}


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def browser_proof(path, validation):
    proof = json.loads(path.read_text())
    require(proof.get('success') is True and proof.get('browser_status') == 'passed', 'Current browser acceptance is incomplete')
    require(not proof.get('page_errors') and not proof.get('unexpected_requests'), 'Browser errors remain')
    require(proof.get('source_sha256') and proof.get('screenshot_sha256'), 'Browser source/screenshots are unbound')
    for name, digest in proof['source_sha256'].items():
        require(sha(ROOT / name) == digest, 'Browser-tested source changed: ' + name)
    for name, digest in proof['screenshot_sha256'].items():
        require(sha(path.parent / name) == digest, 'Browser screenshot changed: ' + name)
    require(set(proof.get('screenshots', [])) == set(proof['screenshot_sha256']), 'Incomplete browser screenshot hashes')
    require(proof.get('build_sha256', {}).get('after') == validation['bundle_sha256'], 'Browser artifact differs from release frontend')
    return proof


def checked_smoke(proof, release, digest, required=COMMON_CHECKS):
    require(proof.get('status') in ('passed', 'passed_with_limitations'), 'Runtime smoke failed: ' + str(proof.get('failure')))
    require(proof.get('release') == release and proof.get('package_runtime_sha256') == digest, 'Smoke does not match release/runtime')
    require(proof.get('disposable_project_removed'), 'Smoke project cleanup missing')
    rows = proof.get('checks', [])
    require(rows and all(row.get('passed') is True for row in rows), 'Runtime smoke has incomplete checks')
    names = {row['name'] for row in rows}
    require(required <= names, 'Missing current runtime checks: ' + ', '.join(sorted(required - names)))
    return names


def checked_upgrade(proof, marker, rollback, release, digest, source):
    names = checked_smoke(proof, release, digest)
    require(proof.get('upgrade_from') == source, 'Wrong source upgrade baseline')
    require({'upgrade_preserves_identity_models_password_graph_and_links',
             upgrade_key_check(source), 'upgrade_preserves_preexisting_graph_marker',
             'source_expected_product_migrations'} <= names, 'Missing upgrade preservation/key migration check')
    if source_has_knowledge_acl(source):
        require('upgrade_existing_kb_policy_preserved' in names, 'Missing existing knowledge permissions preservation check')
        require('image_source_1019_reproduces_binding_failure' in names, 'Missing original historical image failure reproduction')
    for evidence in (marker, rollback):
        require(evidence.get('status') == 'passed' and evidence.get('release') == release, 'Upgrade evidence is incomplete')
        require(evidence.get('source_release', evidence.get('upgrade_from')) == source, 'Upgrade evidence uses a different baseline')
        require(evidence.get('package_runtime_sha256') == digest, 'Upgrade evidence runtime changed')
    require(marker.get('old_graph_seeded_before_backup') and marker.get('graph_preserved_after_upgrade'), 'Old graph was not preserved')
    require(rollback.get('disposable_project_removed') and rollback.get('original_graph_marker'), 'Cold rollback or cleanup incomplete')
    require(rollback.get('source_product_migration_version') is not None and
            rollback.get('source_product_migration_version') == rollback.get('restored_product_migration_version'),
            'Cold rollback did not restore the original product schema')


def collect():
    local = workdir()
    target = Path((local / 'package-path').read_text().strip())
    release = json.loads((target / 'RELEASE.json').read_text())
    validation = validated_proof(local)
    for key in ('gateway_source_sha256', 'ui_source_sha256', 'release_inputs_sha256', 'product_migration_version'):
        require(release.get(key) == validation[key], 'Package built from stale sources: ' + key)
    require(release.get('ui_bundle_sha256') == validation['bundle_sha256'], 'Package frontend is stale')
    require(release.get('validation_sha256') == sha(local / 'validation.json'), 'Build validation evidence changed')
    digest = runtime_digest(target)
    browser_path = Path(os.environ.get('MINDCREEK_CURRENT_BROWSER_PROOF', ROOT / 'docs/plans/evidence/product-details-frontend-2026-10-19.json'))
    browser = browser_proof(browser_path, validation)
    names = ['fresh-smoke/smoke.json', 'archive-check.json', 'database-preflight.json']
    for directory in UPGRADE_BASELINES:
        names.extend(directory + '/' + name for name in ('smoke.json', 'upgrade-marker.json', 'cold-rollback.json'))
    proofs = {name: json.loads((local / name).read_text()) for name in names}
    fresh = proofs['fresh-smoke/smoke.json']
    checked_smoke(fresh, release['release'], digest)
    upgrades = {}
    for directory, source in UPGRADE_BASELINES.items():
        smoke, marker, rollback = [proofs[directory + '/' + name] for name in ('smoke.json', 'upgrade-marker.json', 'cold-rollback.json')]
        checked_upgrade(smoke, marker, rollback, release['release'], digest, source)
        require(marker.get('project') == smoke.get('project') and bool(smoke.get('project')), 'Graph marker belongs to another smoke run')
        expected_version = SOURCE_RELEASES[source]
        require(rollback.get('source_product_migration_version') == expected_version, 'Wrong original migration version')
        require(smoke.get('source_database_facts') and
                smoke['source_database_facts'] == marker.get('source_database_facts') ==
                rollback.get('source_database_facts') == rollback.get('restored_database_facts'),
                'Cold rollback facts do not match this upgrade run')
        require(rollback.get('old_product_addon_images_restored'), 'Original product add-on image recovery incomplete')
        upgrades[source] = {'checks': len(smoke['checks']), 'cold_rollback': 'passed', 'graph_preserved': True}
    archive, database = proofs['archive-check.json'], proofs['database-preflight.json']
    require(archive.get('status') == 'passed' and archive.get('release') == release['release'] and len(archive.get('images', [])) == 9,
            'Nine-image archive verification is incomplete')
    require(database.get('release') == release['release'] and database.get('disposable_container_removed'), 'Database preflight/cleanup incomplete')
    expected_images = {row['name']: row['config_digest'] for row in release['images']}
    require({row['name']: row['config_digest'] for row in archive['images']} == expected_images, 'Archive image configurations changed')
    require(database.get('image', {}).get('config_digest') == expected_images.get('paradedb/paradedb:v0.22.2-pg17'), 'Database preflight used another image')
    limitations = [{'code': 'target_enterprise_acceptance_pending',
                    'impact': 'Real company OAuth/models, physical AMD64 capacity and Word edit/save remain target-environment checks.'}]
    if database.get('status') != 'passed':
        require(database.get('status') == 'failed' and database.get('host_arch') in ('arm64', 'aarch64')
                and 'terminated by signal 6: Aborted' in database.get('diagnostics', '')
                and 'paradedb.score' in database.get('diagnostics', ''),
                'Database failure is not the documented AMD64 emulation limitation')
        limitations.append({'code': 'amd64_emulation_bm25_abort', 'status': 'reproduced_in_this_release',
            'impact': 'BM25/hybrid retrieval is not accepted on this ARM host emulator. Run the included database preflight on physical AMD64 before importing business data; no default index configuration was changed.',
            'evidence': 'database-preflight.json'})
    for value in (fresh, *(proofs[d + '/smoke.json'] for d in UPGRADE_BASELINES)):
        for limitation in value.get('limitations', []):
            if limitation not in limitations:
                limitations.append(limitation)
    report = {'release': release['release'], 'status': 'passed_with_limitations', 'platform': 'linux/amd64',
        'time': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'host_execution': fresh['host_execution'],
        'package_runtime_sha256': digest, 'images': release['images'], 'limitations': limitations,
        'fresh_checks': len(fresh['checks']), 'upgrade_baselines': upgrades,
        'frontend_tests': validation['frontend_tests'], 'python312_tests': validation['python312_tests'],
        'gateway_source_sha256': validation['gateway_source_sha256'], 'ui_source_sha256': validation['ui_source_sha256'],
        'release_inputs_sha256': validation['release_inputs_sha256'], 'validation_sha256': sha(local / 'validation.json'),
        'browser_proof_sha256': sha(browser_path), 'browser_proof_path': str(browser_path),
        'browser_limitations': browser.get('limitations', []), 'product_migration_version': release['product_migration_version'],
        'old_graph_survives_upgrade': True, 'pre_upgrade_cold_rollback_passed': True,
        'checks': fresh['checks'] + [check for d in UPGRADE_BASELINES for check in proofs[d + '/smoke.json']['checks']],
        'evidence_sha256': {name: sha(local / name) for name in names}, 'upstream_clean': True, 'company_deployed': False}
    (local / 'acceptance.json').write_text(json.dumps(report, indent=2) + '\n')
    (ROOT / 'docs/plans/evidence/current-amd64-package.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f"Current bundle: {len(fresh['checks'])} fresh checks and {len(upgrades)} upgrade/rollback baselines accepted; {validation['frontend_tests']} frontend tests")
    return report


if __name__ == '__main__':
    collect()
