"""Offline image repair must be explicit and target the intended installation."""
import importlib.util
import io
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('image_resources', ROOT / 'deploy/enterprise/image_resources.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class ImageResourceTests(unittest.TestCase):
    def test_audit_defaults_to_read_only(self):
        args = m.parse_args(['audit', '--tenant', '7'])
        self.assertEqual(args.limit, 1000)
        self.assertIsNone(m.manifest_input(args))

    def test_apply_needs_explicit_review_and_manifest(self):
        for extra in ([], ['--manifest', '/unused.json'], ['--confirm-reviewed']):
            with self.subTest(extra=extra), patch('sys.stderr', new=io.StringIO()), self.assertRaises(SystemExit):
                m.parse_args(['apply', '--tenant', '7', *extra])

    def test_reject_mismatched_tenant_and_symlink_before_docker(self):
        with tempfile.TemporaryDirectory() as temp:
            file = Path(temp) / 'reviewed.json'
            file.write_text(json.dumps({'tenant_id': 8, 'items': []}))
            args = ['apply', '--tenant', '7', '--confirm-reviewed', '--manifest', str(file)]
            with patch.object(m, 'load_ops') as load, self.assertRaisesRegex(ValueError, 'tenant_id'):
                m.main(args)
            load.assert_not_called()
            file.write_text(json.dumps({'tenant_id': 7, 'items': []}))
            link = Path(temp) / 'link.json'
            link.symlink_to(file)
            with self.assertRaisesRegex(ValueError, 'regular JSON'):
                m.manifest_input(m.parse_args(args[:-1] + [str(link)]))

    def test_apply_passes_reviewed_bytes_through_stdin_after_instance_check(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            file = root / 'reviewed.json'
            raw = b'{"tenant_id":7,"items":[{"reason":"operator-reviewed synthetic fixture"}]}'
            file.write_bytes(raw)
            dep = SimpleNamespace(STATE=root, PROJECT='synthetic')
            ops = SimpleNamespace(selection=lambda: (root, 'synthetic'), deployment=lambda _: dep,
                                  resolved=lambda _: 'config', confirm_instance=lambda d, c: None)
            with patch.object(m, 'ROOT', root), patch.object(m, 'load_ops', return_value=ops), \
                    patch.object(m.subprocess, 'run') as run:
                m.main(['apply', '--tenant', '7', '--confirm-reviewed', '--manifest', str(file)])
            call = run.call_args
            self.assertEqual(call.kwargs['input'], raw)
            self.assertEqual(call.args[0][-5:], ['image-resources', 'apply', '--tenant', '7', '--confirm-reviewed'])
            self.assertNotIn(str(file), call.args[0])

    def test_wrong_instance_cannot_run_repair(self):
        dep = SimpleNamespace(STATE=Path('/other-instance'), PROJECT='synthetic')
        ops = SimpleNamespace(selection=lambda: (Path('/expected-instance'), 'synthetic'), deployment=lambda _: dep)
        with patch.object(m, 'load_ops', return_value=ops), patch.object(m.subprocess, 'run') as run:
            with self.assertRaisesRegex(ValueError, 'do not agree'):
                m.main(['audit', '--tenant', '7'])
        run.assert_not_called()


if __name__ == '__main__':
    unittest.main()
