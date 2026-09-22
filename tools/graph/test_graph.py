import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('deployment', ROOT / 'deploy/graph/graph.py')
module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)


class GraphConfigurationTests(unittest.TestCase):
    def setUp(self):
        self.state = Path('/synthetic/instance')
        self.password = 'a' * 48

    def test_enable_keeps_auth_models_hotfix_ports_and_existing_volumes(self):
        current = {'services': {'gateway': {'image': 'mindcreek-gateway:r4-20260916-identity-diag1',
            'environment': {'MINDCREEK_IDENTITY_EMAIL_CLAIM': 'email'}}, 'tls': {'ports': ['443:443']},
            'app': {'environment': {'LOG_LEVEL': 'warn'}}}, 'volumes': {'existing': {}}}
        result = module.configure(current, self.state, self.password, True)
        self.assertEqual(result['services']['gateway']['image'], module.GATEWAY_IMAGE)
        self.assertEqual(result['services']['tls'], current['services']['tls'])
        self.assertEqual(result['services']['gateway']['environment']['MINDCREEK_IDENTITY_EMAIL_CLAIM'], 'email')
        self.assertEqual(result['services']['app']['environment']['NEO4J_ENABLE'], 'true')
        self.assertIn('existing', result['volumes'])
        self.assertNotIn('neo4j', current['services'])

    def test_repeated_enable_and_disable_are_idempotent_and_keep_password_volume(self):
        enabled = module.configure({}, self.state, self.password, True)
        self.assertEqual(enabled, module.configure(enabled, self.state, self.password, True))
        disabled = module.configure(enabled, self.state, self.password, False)
        self.assertEqual(disabled['services']['neo4j'], enabled['services']['neo4j'])
        self.assertEqual(disabled['volumes'], enabled['volumes'])
        for name in ['gateway', 'installer']:
            self.assertEqual(disabled['services'][name]['environment']['MINDCREEK_GRAPH_ENABLED'], 'false')
        self.assertEqual(module.configure(disabled, self.state, self.password, True), enabled)

    def test_database_is_private_authenticated_offline_and_persistent(self):
        service = module.graph_service(self.password)
        self.assertNotIn('ports', service)
        self.assertEqual(service['volumes'], ['mindcreek_graph_data:/data'])
        self.assertEqual(service['environment']['NEO4J_AUTH'], 'neo4j/' + self.password)
        self.assertNotIn('NEO4J_PLUGINS', service['environment'])
        self.assertNotIn('NEO4JLABS_PLUGINS', service['environment'])
        self.assertEqual(service['environment']['NEO4J_server_http_enabled'], 'false')
        self.assertEqual(service['platform'], 'linux/amd64')

    def test_custom_database_and_capability_mounts_are_not_silently_replaced(self):
        for services in [
            {'neo4j': {'image': 'custom'}},
            {'gateway': {'image': 'custom'}},
            {'gateway': {'environment': {'MINDCREEK_NATIVE_ROUTES_FILE': '/custom.json'}}},
            {'installer': {'environment': {'MINDCREEK_CAPABILITIES_FILE': '/custom.json'}}},
            {'gateway': {'volumes': ['/custom:' + module.ROUTE_TARGET + ':ro']}},
            {'app': {'environment': {'NEO4J_URI': 'bolt://custom:7687'}}},
            {'app': {'environment': {'NEO4J_PASSWORD': 'custom'}}},
            {'gateway': {'volumes': ['/custom:' + module.CAP_TARGET + ':ro']}},
            {'installer': {'volumes': [{'type': 'bind', 'source': '/custom', 'target': module.CAP_TARGET}]}},
        ]:
            with self.subTest(services=services), self.assertRaises(ValueError):
                module.configure({'services': services}, self.state, self.password, True)

    def test_existing_resource_limits_preserved_and_password_mismatch_rejected(self):
        current = module.configure({}, self.state, self.password, True)
        current['services']['neo4j']['mem_limit'] = '3g'
        self.assertEqual(module.configure(current, self.state, self.password, True)['services']['neo4j']['mem_limit'], '3g')
        with self.assertRaises(ValueError): module.configure(current, self.state, 'b' * 48, True)

    def test_atomic_secret_is_private_and_symlink_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'secret'
            module.atomic(target, 'synthetic')
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            link = Path(directory) / 'link'; link.symlink_to(target)
            with self.assertRaises(ValueError): module.atomic(link, 'replacement')
            self.assertEqual(target.read_text(), 'synthetic')

    def test_capability_profile_changes_only_graph(self):
        original = json.loads((ROOT / 'config/r3-capabilities.json').read_text())
        enabled = json.loads(json.dumps(original)); enabled['capabilities']['rag_graph'] = True
        self.assertEqual([key for key in original['capabilities'] if original['capabilities'][key] != enabled['capabilities'][key]], ['rag_graph'])


if __name__ == '__main__': unittest.main()
