#!/usr/bin/env python3
"""Package a validated current release while preserving historical archives."""
import argparse
import json
from pathlib import Path
import shutil

from build_current import sha, validated_proof, workdir
from collect_current import UPGRADE_BASELINES, collect, require
import package_enterprise as package


def prepare_limitation_evidence(report, target):
    """Retain historical diagnostics and resolve every limitation before sealing."""
    history = package.ROOT / 'docs/plans/evidence/enterprise-amd64'
    for name in ('bm25-diagnostic.json', 'bm25-diagnostic-arm64.json'):
        source, destination = history / name, package.LOCAL / name
        require(source.is_file(), 'Missing historical diagnostic: ' + name)
        require(not destination.exists() or sha(destination) == sha(source),
                'Historical diagnostic conflicts with current workdir: ' + name)
        shutil.copy2(source, destination)
    evidence = (target / 'evidence').resolve()
    for limitation in report.get('limitations', []):
        reference = limitation.get('evidence')
        if reference is None:
            continue
        require(isinstance(reference, str) and bool(reference) and not Path(reference).is_absolute(),
                'Invalid limitation evidence reference')
        source = (package.LOCAL / reference).resolve()
        destination = (evidence / reference).resolve()
        require(source.is_relative_to(package.LOCAL.resolve()) and destination.is_relative_to(evidence),
                'Limitation evidence escapes its directory: ' + reference)
        require(source.is_file(), 'Missing limitation evidence: ' + reference)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)
        require(destination.is_file() and sha(destination) == sha(source),
                'Limitation evidence copy failed: ' + reference)


def main():
    package.LOCAL = workdir()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['assemble', 'export', 'finalize'])
    args = parser.parse_args()
    if args.command == 'assemble':
        validation = validated_proof(package.LOCAL)
        build = json.loads((package.LOCAL / 'build.json').read_text())
        require(build.get('validation_sha256') == sha(package.LOCAL / 'validation.json'), 'Build validation changed')
        require(build.get('ui_bundle_sha256') == validation['bundle_sha256'], 'Build frontend changed')
        target = package.assemble()
        record = json.loads((target / 'RELEASE.json').read_text())
        record.update(installation_modes=['fresh', 'upgrade-v0.8-enterprise', 'isolated-cold-restore'],
            features=['R5 employee assistant', 'native knowledge graph browser', 'editable Word export',
                      'workspace administrator member and own API key management', 'knowledge base visibility and document permissions',
                      'permission-scoped Wiki MCP search and read', 'installation-wide configurable service name and logo',
                      'uploaded agent icons', 'enterprise source filters',
                      'knowledge and answer image authorization from trusted parser provenance',
                      'historical image ownership audit and explicit operator-reviewed repair'],
            state_schema='R2-R5 workspace-centric plus workspace roles, knowledge ACL, appearance and reviewed image ownership',
            product_migration_version=validation['product_migration_version'], supported_upgrade_baselines=list(UPGRADE_BASELINES.values()),
            optional_images=['mindcreek-neo4j:2025.10.1-graph1'])
        (target / 'RELEASE.json').write_text(json.dumps(record, indent=2) + '\n')
        return
    target = Path((package.LOCAL / 'package-path').read_text().strip())
    if args.command == 'finalize':
        require(not target.with_suffix('.tar.gz').exists(), 'Final archive already exists')
        report = collect()
        validation = validated_proof(package.LOCAL)
        record = json.loads((target / 'RELEASE.json').read_text())
        record['validated_upgrade_baselines'] = list(report['upgrade_baselines'])
        (target / 'RELEASE.json').write_text(json.dumps(record, indent=2) + '\n')
        evidence = target / 'evidence'
        for directory in UPGRADE_BASELINES:
            shutil.copytree(package.LOCAL / directory, evidence / directory, dirs_exist_ok=True,
                            ignore=lambda path, names: [name for name in names if name not in ('smoke.json', 'upgrade-marker.json', 'cold-rollback.json')])
        shutil.copy2(package.LOCAL / 'fresh-smoke/smoke.json', package.LOCAL / 'smoke.json')
        for name in ('validation.json', 'ui-build.json'):
            shutil.copy2(package.LOCAL / name, evidence / name)
        for name, value in validation['logs'].items():
            shutil.copy2(package.LOCAL / value['path'], evidence / name)
        browser = Path(report['browser_proof_path'])
        destination = target / 'docs/plans/evidence' / browser.name
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(browser, destination)
        for name in json.loads(browser.read_text())['screenshot_sha256']:
            path = (destination.parent / name).resolve()
            require(path.is_relative_to(target.resolve()), 'Browser asset escapes package')
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(browser.parent / name, path)
        prepare_limitation_evidence(report, target)
    getattr(package, args.command)(target)


if __name__ == '__main__':
    main()
