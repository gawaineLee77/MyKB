import json
from pathlib import Path
import tempfile
import unittest

from apply_gateway_hotfix import merge_override, sha, verify_payload


class GatewayHotfixTests(unittest.TestCase):
    def setUp(self):
        self.patch = {'base_gateway_image': 'mindcreek-gateway:base', 'image': {'name': 'mindcreek-gateway:fixed'}}

    def test_merge_keeps_existing_operator_configuration(self):
        before = {'services': {'gateway': {'environment': {'EXAMPLE': 'value'}, 'volumes': ['data:/data']},
                               'tls': {'ports': ['8443:443']}}, 'networks': {'default': {'name': 'company'}}}
        snapshot = json.loads(json.dumps(before))
        result = merge_override(before, self.patch)
        self.assertEqual(before, snapshot)
        self.assertEqual(result['services']['gateway']['environment'], before['services']['gateway']['environment'])
        self.assertEqual(result['services']['gateway']['volumes'], before['services']['gateway']['volumes'])
        self.assertEqual(result['services']['tls'], before['services']['tls'])
        self.assertEqual(result['networks'], before['networks'])
        for name in ['gateway', 'installer']:
            self.assertEqual(result['services'][name]['image'], self.patch['image']['name'])
            self.assertEqual(result['services'][name]['pull_policy'], 'never')

    def test_repeat_application_is_idempotent(self):
        first = merge_override({}, self.patch)
        self.assertEqual(merge_override(first, self.patch), first)

    def test_explicit_predecessor_hotfix_can_be_upgraded(self):
        self.patch['replaces'] = ['mindcreek-gateway:previous-hotfix']
        updated = merge_override({'services': {'gateway': {'image': 'mindcreek-gateway:previous-hotfix'}}}, self.patch)
        self.assertEqual(updated['services']['gateway']['image'], self.patch['image']['name'])

    def test_conflicting_images_and_architectures_require_review(self):
        for name in ['gateway', 'installer']:
            for override in [{'image': 'company/custom:latest'}, {'platform': 'linux/arm64'}]:
                with self.subTest(name=name, override=override), self.assertRaises(ValueError):
                    merge_override({'services': {name: override}}, self.patch)

    def test_payload_tampering_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            file = root / 'image.tar'
            file.write_bytes(b'synthetic image')
            (root / 'SHA256SUMS').write_text(sha(file) + '  image.tar\n')
            verify_payload(root)
            file.write_bytes(b'changed')
            with self.assertRaises(ValueError):
                verify_payload(root)

    def test_checksum_cannot_reference_outside_bundle(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            root = parent / 'patch'
            root.mkdir()
            outside = parent / 'outside'
            outside.write_bytes(b'content')
            (root / 'SHA256SUMS').write_text(sha(outside) + '  ../outside\n')
            with self.assertRaises(ValueError):
                verify_payload(root)


if __name__ == '__main__':
    unittest.main()
