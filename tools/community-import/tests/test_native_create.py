"""Native KB creation is recoverable without replaying uncertain POSTs."""
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from mc_importer.common import ImportFailure
from mc_importer.publish import create_knowledge_base
from mc_importer.workspace import Workspace


class NativeTransport:
    def __init__(self):
        self.rows, self.posts, self.fail = [], 0, None
    def validate_url(self, url): pass
    def headers(self, config): return {"X-API-Key":"synthetic", "X-Tenant-ID":"42"}
    def transfer(self, request, limit, retry_read=False):
        if request.method == "POST":
            self.posts += 1
            if self.fail == "denied": raise ImportFailure("denied", status=403)
            row = {**json.loads(request.data), "id":"kb-synthetic"}
            self.rows.append(row)
            if self.fail == "lost": raise ImportFailure("unavailable")
        else:
            row = self.rows[0] if request.full_url.endswith("/kb-synthetic") else self.rows
        return json.dumps({"success":True, "data":row}).encode(), {}


class NativeCreation(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = {"source_id":"synthetic", "mindcreek":{"base_url":"http://native.invalid"}}
        self.workspace = Workspace(Path(self.temp.name), self.config)
        self.http = NativeTransport()
    def tearDown(self): self.temp.cleanup()
    def create(self): return create_knowledge_base(self.workspace, self.http, "Synthetic KB")
    def test_repeated_confirmed_create_does_not_post(self):
        self.assertTrue(self.create()["created"])
        self.assertFalse(self.create()["created"])
        self.assertEqual(1, self.http.posts)
    def test_restart_reconciles_lost_response(self):
        self.http.fail = "lost"
        with self.assertRaises(ImportFailure): self.create()
        self.workspace = Workspace(Path(self.temp.name), self.config)
        self.assertFalse(self.create()["created"])
        self.assertEqual(1, self.http.posts)
    def test_unknown_result_never_blindly_retries(self):
        self.http.fail = "lost"
        with self.assertRaises(ImportFailure): self.create()
        self.http.rows.clear()
        with self.assertRaises(ImportFailure) as error: self.create()
        self.assertEqual("mindcreek.knowledge_base_create_unresolved", error.exception.code)
        self.assertEqual(1, self.http.posts)
    def test_definitive_rejection_can_retry(self):
        self.http.fail = "denied"
        with self.assertRaises(ImportFailure): self.create()
        self.http.fail = None
        self.assertTrue(self.create()["created"])
        self.assertEqual(2, self.http.posts)
