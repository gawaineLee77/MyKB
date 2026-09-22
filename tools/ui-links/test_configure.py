import copy
import json
from pathlib import Path
import tempfile
import unittest
from configure import merge_override, atomic_json, verify_image, TARGET

IMAGE = 'mindcreek-ui:r4-20260918-links1'

class ConfigureTests(unittest.TestCase):
    def test_classic_and_containerd_image_ids(self):
        expected = {'id': 'sha256:manifest', 'config_digest': 'sha256:config'}
        for digest in expected.values():
            verify_image({'Architecture': 'amd64', 'Os': 'linux', 'Id': digest}, expected)
        for wrong in [{'Architecture': 'arm64', 'Os': 'linux', 'Id': expected['id']},
                      {'Architecture': 'amd64', 'Os': 'linux', 'Id': 'sha256:wrong'}]:
            with self.assertRaises(ValueError):
                verify_image(wrong, expected)

    def test_preserve_and_repeat(self):
        original = {'services': {'gateway': {'image': 'mindcreek-gateway:r4-20260916-graph1'},
                    'app': {'environment': {'NEO4J_ENABLE': 'true'}},
                    'frontend': {'ports': ['127.0.0.1:18080:80'], 'environment': {'DEFAULT_LOCALE': 'zh-CN'}}},
                    'volumes': {'mindcreek_graph_data': {}}}
        snapshot = copy.deepcopy(original)
        state = Path('/srv/synthetic-instance')
        result = merge_override(original, state, IMAGE)
        self.assertEqual(original, snapshot)
        self.assertEqual(result['services']['gateway'], original['services']['gateway'])
        self.assertEqual(result['services']['app'], original['services']['app'])
        self.assertEqual(result['volumes'], original['volumes'])
        self.assertEqual(result['services']['frontend']['ports'], original['services']['frontend']['ports'])
        self.assertEqual(merge_override(result, state, IMAGE), result)

    def test_custom_images_and_mounts_require_review(self):
        for frontend in [{'image': 'custom:ui'}, {'platform': 'linux/arm64'},
                         {'volumes': ['wrong:' + TARGET + ':ro']},
                         {'volumes': [{'target': '/usr/share/nginx/html', 'source': '/custom'}]}]:
            with self.assertRaises(ValueError):
                merge_override({'services': {'frontend': frontend}}, Path('/srv/synthetic'), IMAGE)

    def test_atomic_config_permissions_and_unicode(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'links.json'
            atomic_json(path, {'description': '合成配置'}, 0o644)
            self.assertEqual(json.loads(path.read_text())['description'], '合成配置')
            self.assertEqual(path.stat().st_mode & 0o777, 0o644)
            atomic_json(path, {'description': '新配置'}, 0o600)
            self.assertEqual(json.loads(path.read_text())['description'], '新配置')
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

if __name__ == '__main__':
    unittest.main()
