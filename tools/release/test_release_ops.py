"""Upgrade safety contracts: identity, released overrides, cold backup and recovery."""
import importlib.util
import json
import shutil
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('ops', ROOT/'deploy/enterprise/release_ops.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class ReleaseTests(unittest.TestCase):
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
        good={'upstream_commit':m.UPSTREAM,'platform':'linux/amd64','images':images}
        m.checked_source(good,good)
        bad=json.loads(json.dumps(good));bad['upstream_commit']='legacy'
        with self.assertRaises(ValueError):m.checked_source(bad,good)
        bad=json.loads(json.dumps(good));bad['images'][0]['id']='new-database'
        with self.assertRaises(ValueError):m.checked_source(bad,good)

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
