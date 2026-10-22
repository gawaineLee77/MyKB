"""A current release must not inherit stale validation or a different upgrade proof."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import build_current as build
import collect_current as collect


def frontend_log(count=625):
    return f'ℹ tests {count}\nℹ pass {count}\nℹ fail 0\nℹ cancelled 0\nℹ skipped 0\nℹ todo 0\n'


class SummaryTests(unittest.TestCase):
    def test_target_migration_tracks_product_sources(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            migrations = root / 'services/gateway/internal/database/migrations'
            migrations.mkdir(parents=True)
            (migrations / '000021_appearance.up.sql').write_text('synthetic')
            (migrations / '000022_image_bindings.up.sql').write_text('synthetic')
            with patch.object(build, 'ROOT', root):
                self.assertEqual(build.migration_version(), 22)
            (migrations / '000021_appearance.up.sql').unlink()
            (migrations / '000022_image_bindings.up.sql').unlink()
            with patch.object(build, 'ROOT', root), self.assertRaisesRegex(RuntimeError, 'Missing'):
                build.migration_version()

    def test_actual_frontend_count_not_hardcoded(self):
        self.assertEqual(build.frontend_count(frontend_log(625)), 625)
        self.assertEqual(build.frontend_count(frontend_log(626)), 626)

    def test_incomplete_or_duplicate_summary_rejected(self):
        for log in (frontend_log().replace('pass 625', 'pass 624'), frontend_log() * 2,
                    frontend_log().replace('skipped 0', 'skipped 1'), 'fail 0'):
            with self.subTest(log=log), self.assertRaises(RuntimeError):
                build.frontend_count(log)

    def test_installer_failure_not_accepted_by_old_ok_line(self):
        self.assertEqual(build.installer_count('Ran 51 tests in 1s\n\nOK\n'), 51)
        for log in ('Ran 51 tests in 1s\nFAILED\n', 'Ran 1 tests in 1s\nOK\n', 'OK\n'):
            with self.assertRaises(RuntimeError):
                build.installer_count(log)


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.local = Path(self.temp.name)
        self.ui = self.local / 'ui'
        (self.ui / 'dist').mkdir(parents=True)
        (self.ui / 'dist/index.html').write_text('current build')
        self.fingerprints = {'gateway_source_sha256': 'gateway', 'ui_source_sha256': 'ui',
                             'release_inputs_sha256': 'runtime', 'upstream_commit': 'pinned'}
        self.proof = {'status': 'passed', **self.fingerprints, 'ui_path': str(self.ui),
                      'bundle_sha256': build.bundle_digest(self.ui / 'dist'), 'frontend_tests': 625,
                      'python312_tests': 51, 'logs': {}}
        for name in ('overlay.log', 'frontend-tests.log', 'typecheck.log', 'build.log', 'native-boundary.log',
                     'gateway-tests.log', 'installer-python312.log'):
            content = frontend_log() if name == 'frontend-tests.log' else 'Ran 51 tests in 1s\n\nOK\n'
            path = self.local / name
            path.write_text(content)
            self.proof['logs'][name] = {'path': name, 'sha256': build.sha(path)}
        self.save()
        self.addCleanup(patch.stopall)
        patch.object(build, 'fingerprints', return_value=self.fingerprints).start()
        patch.object(build, 'check_upstream').start()

    def save(self):
        (self.local / 'validation.json').write_text(json.dumps(self.proof))

    def test_validated_proof_uses_bound_current_count(self):
        self.assertEqual(build.validated_proof(self.local)['frontend_tests'], 625)

    def test_changed_sources_reject_all_old_logs(self):
        self.fingerprints['gateway_source_sha256'] = 'edited gateway'
        with self.assertRaisesRegex(RuntimeError, 'stale'):
            build.validated_proof(self.local)

    def test_replaced_log_rejected(self):
        (self.local / 'gateway-tests.log').write_text('old successful test output')
        with self.assertRaisesRegex(RuntimeError, 'log changed'):
            build.validated_proof(self.local)

    def test_missing_validation_rejected(self):
        del self.proof['logs']['native-boundary.log']
        self.save()
        with self.assertRaisesRegex(RuntimeError, 'Incomplete'):
            build.validated_proof(self.local)

    def test_changed_compiled_ui_rejected(self):
        (self.ui / 'dist/index.html').write_text('untested ui')
        with self.assertRaisesRegex(RuntimeError, 'artifact changed'):
            build.validated_proof(self.local)


class UpgradeProofTests(unittest.TestCase):
    def setUp(self):
        self.source = 'current-20261009-a86981ad'
        self.smoke = {'status': 'passed', 'release': 'new', 'package_runtime_sha256': 'runtime',
                      'upgrade_from': self.source, 'disposable_project_removed': True,
                      'checks': [{'name': name, 'passed': True} for name in collect.COMMON_CHECKS | {
                          'upgrade_preserves_identity_models_password_graph_and_links',
                          'upgrade_legacy_machine_key_requires_explicit_mapping',
                          'upgrade_preserves_preexisting_graph_marker', 'source_expected_product_migrations'}]}
        base = {'status': 'passed', 'release': 'new', 'source_release': self.source, 'package_runtime_sha256': 'runtime'}
        self.marker = {**base, 'old_graph_seeded_before_backup': True, 'graph_preserved_after_upgrade': True}
        self.rollback = {**base, 'disposable_project_removed': True, 'original_graph_marker': True,
                         'source_product_migration_version': 16, 'restored_product_migration_version': 16}

    def check(self):
        collect.checked_upgrade(self.smoke, self.marker, self.rollback, 'new', 'runtime', self.source)

    def test_1009_existing_r5_schema_supported(self):
        self.check()

    def use_1019(self):
        self.source = 'current-20261019-d3fafdaf'
        self.smoke['upgrade_from'] = self.source
        self.marker['source_release'] = self.source
        self.rollback.update(source_release=self.source, source_product_migration_version=21,
                             restored_product_migration_version=21)
        self.smoke['checks'] = [c for c in self.smoke['checks']
                                if c['name'] != 'upgrade_legacy_machine_key_requires_explicit_mapping']
        self.smoke['checks'].extend([
            {'name': 'upgrade_existing_machine_key_scope_preserved', 'passed': True},
            {'name': 'upgrade_existing_kb_policy_preserved', 'passed': True},
            {'name': 'image_source_1019_reproduces_binding_failure', 'passed': True},
        ])

    def test_1019_preserves_existing_acl_and_mapped_keys(self):
        self.use_1019()
        self.check()

    def test_1019_cannot_reuse_legacy_key_migration_proof(self):
        self.use_1019()
        self.smoke['checks'] = [c for c in self.smoke['checks']
                                if c['name'] != 'upgrade_existing_machine_key_scope_preserved']
        self.smoke['checks'].append({'name': 'upgrade_legacy_machine_key_requires_explicit_mapping', 'passed': True})
        with self.assertRaisesRegex(RuntimeError, 'key migration'):
            self.check()

    def test_1019_requires_acl_preservation(self):
        self.use_1019()
        self.smoke['checks'] = [c for c in self.smoke['checks'] if c['name'] != 'upgrade_existing_kb_policy_preserved']
        with self.assertRaisesRegex(RuntimeError, 'knowledge permissions'):
            self.check()

    def test_r4_proof_cannot_stand_in_for_1009(self):
        self.marker['source_release'] = 'r4-20260916-118b8af3'
        with self.assertRaisesRegex(RuntimeError, 'different baseline'):
            self.check()

    def test_runtime_edit_or_schema_mismatch_rejected(self):
        self.rollback['restored_product_migration_version'] = 21
        with self.assertRaisesRegex(RuntimeError, 'original product schema'):
            self.check()
        self.rollback['restored_product_migration_version'] = 16
        self.marker['package_runtime_sha256'] = 'old package'
        with self.assertRaisesRegex(RuntimeError, 'runtime changed'):
            self.check()

    def test_missing_new_authorization_check_rejected(self):
        self.smoke['checks'] = [c for c in self.smoke['checks'] if c['name'] != 'current_machine_key_scope_and_role_revocation']
        with self.assertRaisesRegex(RuntimeError, 'Missing current runtime'):
            self.check()


class BrowserProofTests(unittest.TestCase):
    def test_pending_or_different_release_bundle_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'source.ts').write_text('source')
            (root / 'screen.png').write_bytes(b'synthetic fixture')
            path = root / 'browser.json'
            proof = {'success': True, 'browser_status': 'passed', 'source_sha256': {'source.ts': build.sha(root / 'source.ts')},
                     'screenshots': ['screen.png'], 'screenshot_sha256': {'screen.png': build.sha(root / 'screen.png')},
                     'build_sha256': {'after': 'release-bundle'}}
            with patch.object(collect, 'ROOT', root):
                path.write_text(json.dumps(proof))
                collect.browser_proof(path, {'bundle_sha256': 'release-bundle'})
                with self.assertRaisesRegex(RuntimeError, 'differs'):
                    collect.browser_proof(path, {'bundle_sha256': 'changed-bundle'})
                proof['success'] = False
                path.write_text(json.dumps(proof))
                with self.assertRaisesRegex(RuntimeError, 'incomplete'):
                    collect.browser_proof(path, {'bundle_sha256': 'release-bundle'})


if __name__ == '__main__':
    unittest.main()
