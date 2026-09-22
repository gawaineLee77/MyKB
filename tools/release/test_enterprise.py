"""Safety and recovery contracts of the portable installer; no real services."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('enterprise',ROOT/'deploy/enterprise/mindcreek.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory()
        self.root=Path(self.tmp.name)
        self.oldroot,self.oldstate=m.ROOT,m.STATE
        m.ROOT=self.root;m.STATE=self.root/'instance'
        for n in ['enterprise.env.example','compose.json']:
            shutil.copy2(ROOT/'deploy/enterprise'/n,self.root/n)
        (self.root/'RELEASE.json').write_text(json.dumps({'release':'test','images':[]}))

    def tearDown(self):
        m.ROOT,m.STATE=self.oldroot,self.oldstate
        self.tmp.cleanup()

    def test_password_policy_rejects_before_creating_state(self):
        for invalid in ['a'*32,'123456789012','Ab12345','A1'+'a'*31,'  Ab1234567890','Ab1234567890\n']:
            with self.subTest(length=len(invalid)),self.assertRaises(ValueError):m.initialize(invalid)
            self.assertFalse(m.STATE.exists())
        m.initialize('Admin-0123456789!')

    def test_init_secrets_private_random_and_never_overwritten(self):
        m.initialize('Admin-0123456789!')
        for p in [m.STATE/'enterprise.env',m.STATE/'secrets/admin-password']:
            self.assertEqual(p.stat().st_mode & 0o777,0o600)
        v=m.read_env();self.assertEqual(len(v['SYSTEM_AES_KEY'].encode()),32)
        self.assertNotEqual(v['DB_PASSWORD'],v['JWT_SECRET'])
        with self.assertRaises(ValueError):m.initialize('Another-0123456!')
        self.assertEqual((m.STATE/'secrets/admin-password').read_text(),'Admin-0123456789!\n')

    def test_secrets_not_read_when_public_or_symlinked(self):
        m.initialize('Admin-0123456789!')
        p=m.STATE/'enterprise.env';p.chmod(0o644)
        with self.assertRaises(ValueError):m.read_env()
        p.chmod(0o600);p.rename(m.STATE/'actual');p.symlink_to(m.STATE/'actual')
        with self.assertRaises(ValueError):m.read_env()

    def test_env_values_are_literal_and_duplicate_names_rejected(self):
        m.initialize('Admin-0123456789!')
        p=m.STATE/'enterprise.env';p.write_text("SECRET='literal$HOME`command`'\n")
        self.assertEqual(m.read_env()['SECRET'],'literal$HOME`command`')
        p.write_text('KEY=one\nKEY=two\n')
        with self.assertRaises(ValueError):m.read_env()

    def test_shell_settings_do_not_change_app_or_open_registration(self):
        with patch.dict(os.environ,{'LOG_FORMAT':'json','MINDCREEK_INSTALL_REGISTRATION_DISABLED':'false','MINDCREEK_INSTALL_BOOTSTRAP_EMAIL':'attacker@test'}):
            v=m.docker_env({'LOG_LEVEL':'info'})
        self.assertNotIn('LOG_FORMAT',v)
        self.assertEqual(v['MINDCREEK_INSTALL_REGISTRATION_DISABLED'],'true')
        self.assertEqual(v['MINDCREEK_INSTALL_BOOTSTRAP_EMAIL'],'')
        self.assertEqual(m.docker_env({}, {'MINDCREEK_INSTALL_REGISTRATION_DISABLED':'false'})['MINDCREEK_INSTALL_REGISTRATION_DISABLED'],'false')

    def test_failed_prepare_always_closes_registration_window(self):
        m.initialize('Admin-0123456789!')
        with patch.object(m,'validate'),patch.object(m,'verify_images'),patch.object(m,'compose',return_value='private diagnostics'),patch.object(m,'up_services') as up,patch.object(m,'install_command',side_effect=[json.dumps({'stage':'new'}),subprocess.CalledProcessError(1,['synthetic-prepare'])]):
            with self.assertRaises(subprocess.CalledProcessError):m.install(['admin'])
            self.assertEqual(up.call_args_list[-1].args,(['app'],))
            self.assertEqual(up.call_args_list[-1].kwargs,{'recreate':True})
        self.assertEqual((m.STATE/'installation-error.log').stat().st_mode & 0o777,0o600)

    def test_image_verification_accepts_only_locked_amd64_ids(self):
        (m.ROOT/'RELEASE.json').write_text(json.dumps({'release':'test','images':[{'name':'image:tag','id':'manifest','config_digest':'config'}]}))
        for identifier in ['manifest','config']:
            with patch.object(m,'run',return_value=json.dumps([{'Id':identifier,'Os':'linux','Architecture':'amd64'}])):m.verify_images()
        for identifier,architecture in [('other','amd64'),('manifest','arm64')]:
            with patch.object(m,'run',return_value=json.dumps([{'Id':identifier,'Os':'linux','Architecture':architecture}])),self.assertRaises(ValueError):m.verify_images()

    def test_checksum_file_cannot_escape_bundle(self):
        (m.ROOT/'SHA256SUMS').write_text('invalid  ../outside\n')
        with self.assertRaises(ValueError):m.verify_files()

    def test_only_one_shot_installer_can_write_member_secret(self):
        services=json.loads((ROOT/'deploy/enterprise/compose.json').read_text())['services']
        for name,mode in [('gateway',':ro'),('installer',':rw')]:
            mount=next(v for v in services[name]['volumes'] if '/run/mindcreek-enterprise:' in v)
            self.assertTrue(mount.endswith(mode))
        self.assertEqual(services['installer']['profiles'],['tools'])
        self.assertEqual(services['installer']['restart'],'no')

    def test_oidc_internal_discovery_and_jwks_do_not_require_public_origin(self):
        env=json.loads((ROOT/'deploy/enterprise/compose.json').read_text())['services']['app']['environment']
        for name in ['DISCOVERY_URL','JWKS_URI','AUTHORIZATION_ENDPOINT','TOKEN_ENDPOINT','USER_INFO_ENDPOINT']:
            self.assertTrue(env['OIDC_AUTH_'+name].startswith('http://gateway:8080/api/v1/mindcreek/oidc/'))
        self.assertIn('${MINDCREEK_EXTERNAL_ORIGIN',env['OIDC_AUTH_ISSUER_URL'])


if __name__=='__main__':unittest.main()
