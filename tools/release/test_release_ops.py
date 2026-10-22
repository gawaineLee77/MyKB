"""Upgrade safety contracts: identity, released overrides, cold backup and recovery."""
import importlib.util
import base64
import json
import os
import shutil
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('ops', ROOT/'deploy/enterprise/release_ops.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class ReleaseTests(unittest.TestCase):
    def delegation_instance(self, directory):
        spec = importlib.util.spec_from_file_location('delegation_fixture', ROOT/'deploy/enterprise/mindcreek.py')
        dep = importlib.util.module_from_spec(spec); spec.loader.exec_module(dep)
        dep.STATE = Path(directory)
        dep.release = lambda: {'product_migration_version': 21}
        (dep.STATE/'secrets').mkdir(mode=0o700)
        (dep.STATE/'enterprise.env').write_text('# Keep operator comments\nMINDCREEK_UID='+str(os.getuid())+'\nKEEP=literal$&value\n')
        (dep.STATE/'enterprise.env').chmod(0o600)
        return dep

    def test_delegation_plan_is_read_only_and_apply_preserves_existing_settings(self):
        with tempfile.TemporaryDirectory() as directory:
            dep = self.delegation_instance(directory)
            original = (dep.STATE/'enterprise.env').read_bytes()
            self.assertEqual(len(m.delegation_configuration(dep)), 2)
            self.assertEqual((dep.STATE/'enterprise.env').read_bytes(), original)
            self.assertFalse((dep.STATE/'secrets/workspace-admin-key').exists())
            m.delegation_configuration(dep, apply=True)
            key = dep.STATE/'secrets/workspace-admin-key'
            first = key.read_bytes()
            self.assertEqual(len(base64.b64decode(first.strip(), validate=True)), 32)
            self.assertEqual(key.stat().st_mode & 0o777, 0o600)
            self.assertTrue((dep.STATE/'enterprise.env').read_bytes().startswith(original))
            self.assertEqual(dep.read_env()[m.DELEGATION_KEY_SETTING], m.DELEGATION_KEY_PATH)
            self.assertEqual(m.delegation_configuration(dep, apply=True), [])
            self.assertEqual(key.read_bytes(), first)

    def test_existing_unconfigured_key_is_reused_and_blank_setting_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            dep = self.delegation_instance(directory)
            key = dep.STATE/'secrets/workspace-admin-key'
            original = base64.b64encode(b'x'*32)+b'\n'
            key.write_bytes(original); key.chmod(0o600)
            with (dep.STATE/'enterprise.env').open('a') as stream:
                stream.write(m.DELEGATION_KEY_SETTING+'=\n')
            self.assertEqual(len(m.delegation_configuration(dep, apply=True)), 1)
            self.assertEqual(key.read_bytes(), original)
            self.assertEqual(dep.read_env()[m.DELEGATION_KEY_SETTING], m.DELEGATION_KEY_PATH)

    def test_missing_configured_or_invalid_key_is_never_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            dep = self.delegation_instance(directory)
            env = dep.STATE/'enterprise.env'
            with env.open('a') as stream:
                stream.write(m.DELEGATION_KEY_SETTING+'='+m.DELEGATION_KEY_PATH+'\n')
            original = env.read_bytes()
            key = dep.STATE/'secrets/workspace-admin-key'
            with self.assertRaisesRegex(ValueError, 'restore it'):
                m.delegation_configuration(dep, apply=True)
            self.assertFalse(key.exists())
            for value in (b'invalid', base64.b64encode(b'too-short')):
                key.write_bytes(value); key.chmod(0o600)
                with self.assertRaises(ValueError): m.delegation_configuration(dep, apply=True)
                self.assertEqual(key.read_bytes(), value)
            key.unlink(); key.symlink_to(env)
            with self.assertRaisesRegex(ValueError, 'symlink'): m.delegation_configuration(dep, apply=True)
            self.assertEqual(env.read_bytes(), original)

    def test_upgrade_backs_up_old_settings_before_adding_key_and_retains_failure_marker(self):
        with tempfile.TemporaryDirectory() as directory:
            dep = self.delegation_instance(directory)
            actual = dep.STATE
            original = (actual/'enterprise.env').read_bytes()
            dep.verify_images = lambda: None
            dep.validate = lambda: self.assertTrue((dep.STATE/'secrets/workspace-admin-key').exists())
            report = {'access_review_required': m.access_review(dep.release())}
            def backup(old, destination):
                self.assertEqual((actual/'enterprise.env').read_bytes(), original)
                self.assertFalse((actual/'secrets/workspace-admin-key').exists())
                return {'running_services': []}
            def fail_start(current, services):
                self.assertTrue((actual/'secrets/workspace-admin-key').exists())
                self.assertTrue((actual/'upgrade-pending.json').exists())
                raise ValueError('synthetic startup failure')
            with patch.object(m, 'plan', return_value=(dep, {}, report)), patch.object(m, 'backup', side_effect=backup), patch.object(m, 'refresh_policies'), patch.object(m, 'start', side_effect=fail_start):
                with self.assertRaisesRegex(ValueError, 'synthetic startup failure'):
                    m.upgrade(dep, Path('/old'), actual/'backup')
            self.assertEqual(dep.STATE, actual)
            self.assertTrue((actual/'upgrade-pending.json').exists())
            self.assertFalse((actual/'active-release.json').exists())

    def test_access_review_is_required_only_for_acl_releases(self):
        self.assertEqual(m.access_review({}), [])
        self.assertEqual(m.access_review({'product_migration_version': 17}), [])
        self.assertTrue(m.access_review({'product_migration_version': 21}))

    def test_known_addons_unpinned_and_ui_links_preserved(self):
        state=Path('/instance')
        current={'services':{'gateway':{'image':'mindcreek-gateway:r4-20260916-graph1','volumes':['/instance/graph/routes.json:/etc/mindcreek/r3-routes.json:ro']},
            'frontend':{'image':'mindcreek-ui:r4-20260918-links1','volumes':[{'source':'/instance/ui-links','target':'/usr/share/nginx/html/mindcreek-runtime'}],'mem_limit':'1g'}}}
        updated,changes=m.override_plan(current,state,{'images':[]},{})
        self.assertNotIn('image',updated['services']['gateway'])
        self.assertNotIn('image',updated['services']['frontend'])
        self.assertEqual(updated['services']['frontend']['volumes'],current['services']['frontend']['volumes'])
        self.assertEqual(updated['services']['frontend']['mem_limit'],'1g')
        self.assertIn('image',current['services']['frontend'])
        self.assertTrue(changes)

    def test_unknown_image_policy_and_frontend_mount_fail_closed(self):
        for service in [{'image':'custom:1'}, {'volumes':['/custom:/etc/mindcreek/r3-routes.json:ro']}]:
            with self.assertRaises(ValueError):m.override_plan({'services':{'gateway':service}},Path('/s'),{'images':[]},{})
        with self.assertRaises(ValueError):m.override_plan({'services':{'frontend':{'volumes':['/x:/usr/share/nginx/html:ro']}}},Path('/s'),{'images':[]},{})

    def test_graph_password_and_operator_limits_are_preserved(self):
        password='synthetic-existing'
        current={'services':{'neo4j':{'image':m.GRAPH_IMAGE,'labels':{'com.mindcreek.graph':'1'},'environment':{'NEO4J_AUTH':'neo4j/'+password},'mem_limit':'2g'},'app':{'environment':{'NEO4J_ENABLE':'true','NEO4J_PASSWORD':password}}}}
        updated,_=m.override_plan(current,Path('/s'),{'images':[]},{})
        self.assertEqual(updated['services']['gateway']['environment']['MINDCREEK_GRAPH_VIEW_PASSWORD'],password)
        self.assertEqual(updated['services']['neo4j'],current['services']['neo4j'])
        current['services']['neo4j']['environment']['NEO4J_AUTH']='different'
        with self.assertRaises(ValueError):m.override_plan(current,Path('/s'),{'images':[]},{})

    def test_legacy_and_database_changes_are_rejected(self):
        images=[{'name':p+'test','id':'same'} for p in ('paradedb/','redis:','wechatopenai/weknora-app:','wechatopenai/weknora-docreader:')]
        good={'upstream_commit':m.UPSTREAM,'platform':'linux/amd64','images':images,'product_migration_version':21}
        m.checked_source(good,good)
        bad=json.loads(json.dumps(good));bad['upstream_commit']='legacy'
        with self.assertRaises(ValueError):m.checked_source(bad,good)
        bad=json.loads(json.dumps(good));bad['images'][0]['id']='new-database'
        with self.assertRaises(ValueError):m.checked_source(bad,good)

    def test_upgrade_rejects_downgrade_and_unknown_schema_without_touching_data(self):
        images=[{'name':p+'test','id':'same'} for p in ('paradedb/','redis:','wechatopenai/weknora-app:','wechatopenai/weknora-docreader:')]
        latest={'upstream_commit':m.UPSTREAM,'platform':'linux/amd64','images':images,'product_migration_version':21}
        for release in m.HISTORICAL_MIGRATIONS:
            previous={k:v for k,v in latest.items() if k!='product_migration_version'}
            previous['release']=release
            m.checked_source(previous,latest)
            with self.assertRaisesRegex(ValueError,'downgrade'):m.checked_source(latest,previous)
        unknown={k:v for k,v in latest.items() if k!='product_migration_version'}
        with self.assertRaisesRegex(ValueError,'Unknown product migration'):m.checked_source(unknown,latest)

    def test_explicit_instance_and_project_are_required(self):
        for env in ({},{'MINDCREEK_STATE_DIR':'relative','MINDCREEK_PROJECT':'test'},{'MINDCREEK_STATE_DIR':'/tmp/test','MINDCREEK_PROJECT':'--bad'}):
            with patch.dict(m.os.environ,env,clear=True),self.assertRaises(ValueError):m.selection()

    def test_backup_does_not_recurse_into_instance(self):
        dep=type('Dep',(),{'STATE':Path('/tmp/instance').resolve()})()
        with self.assertRaises(ValueError):m.backup_destination(dep,Path('/tmp/instance/copy'))
        self.assertEqual(m.backup_destination(dep,Path('/tmp/instance/backup/test').resolve()),Path('/tmp/instance/backup/test').resolve())

    def test_restore_rejects_incomplete_before_docker(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp);(path/'INCOMPLETE.json').write_text('{}')
            with self.assertRaises(ValueError):m.restore(None,path)

    def test_atomic_json_is_private_and_rejects_symlink(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/'test.json';m.atomic_json(path,{'secret':'synthetic'})
            self.assertEqual(path.stat().st_mode&0o777,0o600)
            path.unlink();path.symlink_to(Path(temp)/'other')
            with self.assertRaises(ValueError):m.atomic_json(path,{})

    def test_graph_configuration_repeats_without_password_rotation(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp); state=root/'state';state.mkdir()
            (root/'bin').mkdir();(root/'config').mkdir()
            shutil.copy2(ROOT/'deploy/graph/graph.py',root/'bin/graph_support.py')
            for name in ('r3-routes.json','r3-capabilities.json'):
                shutil.copy2(ROOT/'config'/name,root/'config'/name)
            def private(path):
                if path.is_symlink() or path.stat().st_mode&0o077:raise ValueError('private file required')
            dep=type('Deployment',(),{'ROOT':root,'STATE':state,'private':staticmethod(private)})()
            m.graph(dep,True)
            password=(state/'graph/password').read_bytes()
            original=json.loads((state/'compose.override.json').read_text())
            m.graph(dep,True)
            self.assertEqual((state/'graph/password').read_bytes(),password)
            self.assertEqual(json.loads((state/'compose.override.json').read_text()),original)
            self.assertEqual(original['services']['app']['depends_on']['neo4j']['condition'],'service_healthy')
            self.assertNotIn('image',original['services']['gateway'])
            m.graph(dep,False)
            self.assertEqual((state/'graph/password').read_bytes(),password)
            self.assertFalse(json.loads((state/'graph/capabilities.json').read_text())['capabilities']['rag_graph'])

if __name__=='__main__':unittest.main()
