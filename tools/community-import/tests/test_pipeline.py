"""Synthetic end-to-end tests; never use live corporate documents or endpoints."""

from __future__ import annotations

import base64
import contextlib
import copy
import io
import json
import os
import socket
import subprocess
import sys
import tempfile
import threading
import unittest
import urllib.parse
from email import policy
from email.parser import BytesParser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from mc_importer.cli import fetch, main
from mc_importer.common import ImportFailure, load_config, read_json, resolve_inside, safe_name, save_json, workspace_lock
from mc_importer.content import Images, markdown, rich_html
from mc_importer.publish import Publisher
from mc_importer.source import fetch_articles, local_articles
from mc_importer.transport import S3Downloader, Transport
from mc_importer.workspace import Workspace

PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jZ1sAAAAASUVORK5CYII=")


def article(code="A-100", kind=1, **values):
    result = {"articleCode": code, "aritcleTitle": "合成数据库排障", "articleAbstract": "非真实业务内容",
              "content": '<h2>处理流程</h2><p>合成步骤</p><img src="image-1" alt="流程图">',
              "contentType": kind, "attachments": [{"fileExtension": "txt", "s3uuid": "attachment-1"}],
              "contentFileUuid": "main-1", "contentFileExtension": "pdf",
              "lastUpdateDate": "2026-09-11T10:00:00Z", "kmsUrl": "https://community.example.invalid/posts/" + code}
    result.update(values)
    return result


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, data, status=200, content_type="application/json"):
        payload = data if isinstance(data, bytes) else json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self):
        parsed = urllib.parse.urlsplit(self.path)
        query = urllib.parse.parse_qs(parsed.query)
        state = self.server.fixture
        state["reads"] += 1
        if parsed.path == "/articles":
            if state.get("article_error"):
                return self.send({"error": "synthetic"}, 503)
            page = int(query.get("page", [1])[0])
            size = int(query.get("size", [2])[0])
            offset = (page - 1) * size if not state.get("repeat_pages") else 0
            return self.send({"data": {"items": state["articles"][offset:offset + size]}})
        if parsed.path == "/detail":
            return self.send({"data": state["articles"][0]})
        if parsed.path == "/signed":
            return self.send({"data": {"url": state["base"] + "/file?signature=synthetic-private"}})
        if parsed.path == "/file":
            state["signed_auth"] = self.headers.get("Authorization")
            return self.send(PNG, content_type="image/png")
        if parsed.path == "/redirect":
            self.send_response(302)
            self.send_header("Location", state["base"] + "/file")
            self.end_headers()
            return
        if parsed.path == "/s3":
            resource = query.get("uuid", [""])[0]
            state["downloads"][resource] = state["downloads"].get(resource, 0) + 1
            if resource not in state["resources"]:
                return self.send({"error": "synthetic_missing"}, 404)
            return self.send(state["resources"][resource], content_type="application/octet-stream")
        if parsed.path.startswith("/api/v1/knowledge/") or parsed.path.endswith("/knowledge"):
            if self.headers.get("X-API-Key") != "synthetic-import-key" or state.get("deny"):
                return self.send({"success": False}, 403)
            tail = parsed.path.removeprefix("/api/v1/knowledge/") if parsed.path.startswith("/api/v1/knowledge/") else ""
            if tail:
                if tail not in state["documents"]:
                    return self.send({"success": False}, 404)
                document = state["documents"][tail]
                if document["parse_status"] == "pending":
                    document["parse_status"] = "failed" if state.get("fail_parse") else "completed"
                return self.send({"success": True, "data": document})
            items = list(state["documents"].values())
            page = int(query.get("page", [1])[0])
            size = int(query.get("page_size", [100])[0])
            return self.send({"success": True, "data": items[(page - 1) * size:page * size],
                              "total": len(items), "page": page, "page_size": size})
        self.send({"error": "synthetic_not_found"}, 404)

    def do_POST(self):
        state = self.server.fixture
        data = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if self.path == "/post-s3":
            value = json.loads(data) if "application/json" in self.headers.get("Content-Type", "") else {
                key: values[0] for key, values in urllib.parse.parse_qs(data.decode()).items()}
            state["post_body"] = value
            return self.send(state["resources"][value["id"]], content_type="application/octet-stream")
        if not (self.path.endswith("/knowledge/file") or self.path.startswith("/api/v1/knowledge/")):
            return self.send({"success": False}, 404)
        if self.headers.get("X-API-Key") != "synthetic-import-key" or state.get("deny_upload"):
            return self.send({"success": False}, 403)
        if self.path.endswith("/reparse"):
            identity = self.path.split("/")[-2]
            state["documents"][identity]["parse_status"] = "pending"
            state["retries"] += 1
            return self.send({"success": True, "data": state["documents"][identity]}, 202)
        message = BytesParser(policy=policy.default).parsebytes(
            ("Content-Type: " + self.headers["Content-Type"] + "\r\nMIME-Version: 1.0\r\n\r\n").encode() + data)
        part = next(message.iter_parts())
        payload = part.get_payload(decode=True)
        identity = str(len(state["documents"]) + state["uploads"] + 1)
        document = {"id": identity, "knowledge_base_id": self.path.split("/")[-3],
                    "file_name": part.get_filename(), "file_size": len(payload), "parse_status": "pending"}
        state["documents"][identity] = document
        state["uploads"] += 1
        if state.get("drop_upload_response"):
            state["drop_upload_response"] = False
            self.connection.shutdown(socket.SHUT_RDWR)
            self.connection.close()
            return
        self.send({"success": True, "data": document}, 201)

    def do_DELETE(self):
        state = self.server.fixture
        if self.headers.get("X-API-Key") != "synthetic-import-key":
            return self.send({"success": False}, 403)
        identity = self.path.split("/")[-1]
        if identity not in state["documents"]:
            return self.send({"success": False}, 404)
        del state["documents"][identity]
        state["deletes"].append(identity)
        self.send({"success": True}, 202)


class PipelineTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="mindcreek-import-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.environment = patch.dict(os.environ, {"TEST_KEY": "synthetic-import-key", "TEST_SOURCE": "Bearer synthetic-source",
                                                  "NO_PROXY": "127.0.0.1,localhost", "no_proxy": "127.0.0.1,localhost"})
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.server.fixture = {"base": self.base, "articles": [article(), article("A-101", 2),
                                  article("A-102", 3, content="## 合成 Markdown\n![图](image-1)")],
                               "resources": {"image-1": PNG, "attachment-1": b"synthetic attachment sentinel",
                                             "main-1": b"%PDF-1.4\nsynthetic main document\n%%EOF"},
                               "reads": 0, "downloads": {}, "documents": {}, "uploads": 0, "retries": 0, "deletes": []}
        self.config = {"schema_version": 1, "source_id": "synthetic-community", "output_dir": str(self.root / "output"),
                       "http": {"allow_insecure_http": True, "read_attempts": 1, "timeout_seconds": 2},
                       "articles": {"request": {"url": self.base + "/articles", "query": {"page": "{page}", "size": "{page_size}"}},
                                    "items_path": "data.items", "pagination": {"mode": "page", "page_size": 2}},
                       "s3": {"request": {"url": self.base + "/s3", "query": {"uuid": "{s3uuid}"},
                                           "headers_env": {"Authorization": "TEST_SOURCE"}}},
                       "mindcreek": {"base_url": self.base, "knowledge_base_id": "kb-a", "headers_env": {"X-API-Key": "TEST_KEY"}}}
        self.http = Transport(self.config)
        self.downloader = S3Downloader(self.http, self.config)
        self.workspace = Workspace(self.root / "work", self.config)

    def prepare(self, articles=None, **kwargs):
        return self.workspace.prepare(articles or self.server.fixture["articles"], self.downloader, **kwargs)

    def test_three_types_and_batch_resume(self):
        articles = list(fetch_articles(self.config, self.http))
        self.assertEqual(len(articles), 3)
        counts = self.prepare(articles)
        self.assertEqual(counts["prepared"], 3)
        self.assertEqual(counts["failed"], 0)
        main = self.workspace.manifest(self.workspace.state["articles"][safe_name("A-100")])
        file = resolve_inside(self.workspace.root, self.workspace.state["articles"][safe_name("A-100")]["manifest"]).parent / main["artifacts"][0]["path"]
        self.assertIn("data:image/png;base64,", file.read_text())
        self.assertIn("合成数据库排障", file.read_text())
        self.assertEqual(len(self.workspace.desired()), 5)  # 3 post/index files + shared attachment + PDF.
        downloads = dict(self.server.fixture["downloads"])
        self.assertEqual(self.prepare(articles)["unchanged"], 3)
        self.assertEqual(downloads, self.server.fixture["downloads"])
        publisher = Publisher(self.workspace, self.http)
        result = publisher.upload(wait_seconds=2)
        self.assertEqual(result["completed"], 5)
        self.assertEqual(result["failed"], 0)
        self.assertEqual(self.server.fixture["uploads"], 5)
        self.assertEqual(publisher.upload()["completed"], 5)
        self.assertEqual(self.server.fixture["uploads"], 5)

    def test_failed_download_keeps_previous_version_and_blocks_prune(self):
        self.prepare([article()])
        old = self.workspace.state["articles"][safe_name("A-100")]["manifest"]
        changed = article(content='<img src="missing-image">', lastUpdateDate="2026-09-12")
        self.assertEqual(self.prepare([changed])["failed"], 1)
        self.assertEqual(self.workspace.state["articles"][safe_name("A-100")]["manifest"], old)
        self.assertTrue(resolve_inside(self.workspace.root, old).exists())
        with self.assertRaisesRegex(ImportFailure, "article_failure_blocks"):
            Publisher(self.workspace, self.http).prune(apply=True)
        self.assertFalse(self.server.fixture["deletes"])

    def test_replacement_prune_only_after_ready_and_preserves_shared_attachment(self):
        self.prepare()
        publisher = Publisher(self.workspace, self.http)
        publisher.upload()
        old_records = set(item["id"] for item in publisher.records.values())
        self.prepare([article(content="<p>new synthetic revision</p>", lastUpdateDate="2026-09-12")])
        with self.assertRaisesRegex(ImportFailure, "replacement_not_ready"):
            publisher.prune(apply=True)
        publisher.upload()
        plan = publisher.prune()
        self.assertEqual(len(plan["candidates"]), 1)
        self.assertFalse(self.server.fixture["deletes"])
        result = publisher.prune(apply=True)
        self.assertEqual(result["deletion_requested"], 1)
        self.assertTrue(set(self.server.fixture["deletes"]).issubset(old_records))
        self.assertEqual(len(self.server.fixture["documents"]), 5)

    def test_uncertain_write_is_reconciled_not_reuploaded_or_owned(self):
        self.prepare([article(attachments=[], content="<p>synthetic</p>")])
        publisher = Publisher(self.workspace, self.http)
        self.server.fixture["drop_upload_response"] = True
        self.assertEqual(publisher.upload()["failed"], 1)
        self.assertEqual(self.server.fixture["uploads"], 1)
        publisher.upload()
        self.assertEqual(self.server.fixture["uploads"], 1)
        record = next(iter(publisher.records.values()))
        self.assertTrue(record["reconciled"])
        self.assertFalse(record["owned"])

    def test_rejected_upload_can_be_retried_after_configuration_fix(self):
        self.prepare([article(attachments=[], content="<p>synthetic</p>")])
        publisher = Publisher(self.workspace, self.http)
        self.server.fixture["deny_upload"] = True
        self.assertEqual(publisher.upload()["failed"], 1)
        self.assertFalse(next(iter(publisher.records.values()))["attempted"])
        self.server.fixture["deny_upload"] = False
        self.assertEqual(publisher.upload()["completed"], 1)

    def test_permissions_checked_even_for_cached_completed_files(self):
        self.prepare([article()])
        publisher = Publisher(self.workspace, self.http)
        publisher.upload()
        self.server.fixture["deny"] = True
        with self.assertRaises(ImportFailure) as error:
            publisher.upload()
        self.assertEqual(error.exception.status, 403)

    def test_failed_parse_needs_explicit_retry(self):
        self.prepare([article(attachments=[], content="<p>synthetic</p>")])
        publisher = Publisher(self.workspace, self.http)
        self.server.fixture["fail_parse"] = True
        self.assertEqual(publisher.upload()["failed"], 1)
        self.assertEqual(publisher.upload()["failed"], 1)
        self.assertEqual(self.server.fixture["retries"], 0)
        self.server.fixture["fail_parse"] = False
        self.assertEqual(publisher.upload(retry_failed=True)["completed"], 1)
        self.assertEqual(self.server.fixture["retries"], 1)

    def test_target_knowledge_bases_do_not_share_upload_ledger(self):
        self.prepare([article(attachments=[], content="<p>synthetic</p>")])
        Publisher(self.workspace, self.http).upload()
        self.config["mindcreek"]["knowledge_base_id"] = "kb-b"
        result = Publisher(self.workspace, self.http).upload()
        self.assertEqual(result["completed"], 1)
        self.assertEqual(self.server.fixture["uploads"], 2)
        self.assertEqual(len(self.workspace.state["uploads"]), 2)

    def test_fetch_snapshot_is_atomic_when_pagination_fails(self):
        output = fetch(self.workspace, self.http)
        before = output.read_bytes()
        self.server.fixture["repeat_pages"] = True
        with self.assertRaisesRegex(ImportFailure, "repeated_page"):
            fetch(self.workspace, self.http)
        self.assertEqual(output.read_bytes(), before)

    def test_signed_url_allowlist_and_no_credential_forwarding(self):
        self.config["s3"].update({"response_mode": "json_url", "url_path": "data.url", "download_origins": [self.base]})
        self.config["s3"]["request"]["url"] = self.base + "/signed"
        destination = self.root / "download"
        S3Downloader(self.http, self.config).download("image-1", destination, 4096)
        self.assertEqual(destination.read_bytes(), PNG)
        self.assertIsNone(self.server.fixture["signed_auth"])
        self.config["s3"]["download_origins"] = []
        with self.assertRaisesRegex(ImportFailure, "origin_denied"):
            S3Downloader(self.http, self.config).download("image-1", destination, 4096)

    def test_redirects_and_content_selected_urls_are_denied(self):
        with self.assertRaisesRegex(ImportFailure, "uuid_invalid"):
            self.downloader.download("http://127.0.0.1/private", self.root / "download", 1000)
        with self.assertRaises(ImportFailure) as error:
            self.http.json_request({"url": self.base + "/redirect", "headers_env": {"Authorization": "TEST_SOURCE"}}, {})
        self.assertEqual(error.exception.status, 302)

    def test_size_limits_and_file_magic_fail_closed(self):
        with self.assertRaisesRegex(ImportFailure, "too_large"):
            self.downloader.download("image-1", self.root / "download", 4)
        self.server.fixture["resources"]["main-1"] = b'{"error":"synthetic unauthorized"}'
        self.assertEqual(self.prepare([article(kind=2)])["failed"], 1)
        self.assertEqual(self.workspace.state["articles"][safe_name("A-100")]["error"]["code"], "document.signature_mismatch")

    def test_html_sanitizes_scripts_attributes_and_embeds_uuid_images(self):
        images = Images(self.root / "html", self.downloader, {})
        value = rich_html('<script>private()</script><p onclick="private()">keep</p><img src=image-1 onerror="private()"><table><tr><td colspan="2">cell</td></tr></table>', images)
        self.assertNotIn("private", value)
        self.assertNotIn("onerror", value)
        self.assertIn('colspan="2"', value)
        self.assertIn("data:image/png;base64,", value)

    def test_markdown_image_forms_and_code_examples(self):
        images = Images(self.root / "markdown", self.downloader, {})
        value = markdown('![a](image-1)\n![b][ref]\n![ref]\n[ref]: image-1\n<img src="image-1">\n`![example](not-real)`\n```md\n![example](not-real)\n```\n', images)
        self.assertEqual(value.count("data:image/png;base64,"), 4)
        self.assertIn("![example](not-real)", value)
        self.assertEqual(self.server.fixture["downloads"]["image-1"], 1)

    def test_unsupported_attachment_is_explicit_and_optionally_archived(self):
        self.server.fixture["resources"]["archive"] = b"synthetic zip"
        value = article(attachments=[{"s3uuid": "archive", "fileExtension": "zip"}])
        self.assertEqual(self.prepare([value])["failed"], 1)
        self.config["unsupported_attachments"] = "archive"
        counts = self.prepare([value])
        self.assertEqual(counts["prepared"], 1)
        self.assertEqual(counts["warnings"], 1)
        self.assertEqual(len(self.workspace.desired()), 1)

    def test_main_file_repeated_as_attachment_downloaded_once(self):
        value = article(kind=2, attachments=[{"fileExtension": "pdf", "s3uuid": "main-1"}])
        self.assertEqual(self.prepare([value])["prepared"], 1)
        self.assertEqual(self.server.fixture["downloads"]["main-1"], 1)
        self.assertEqual(len(self.workspace.desired()), 2)

    def test_path_confinement_hash_verification_and_private_files(self):
        self.prepare([article("../../outside", attachments=[])])
        with self.assertRaisesRegex(ImportFailure, "path_denied"):
            resolve_inside(self.workspace.root, "../../outside")
        entry = next(iter(self.workspace.state["articles"].values()))
        manifest_path = resolve_inside(self.workspace.root, entry["manifest"])
        self.assertEqual(manifest_path.stat().st_mode & 0o777, 0o600)
        manifest = self.workspace.manifest(entry)
        body = manifest_path.parent / manifest["artifacts"][0]["path"]
        body.write_text("changed synthetic bytes")
        with self.assertRaisesRegex(ImportFailure, "artifact_changed"):
            self.workspace.desired()

    def test_workspace_lock_releases_and_symlink_is_rejected(self):
        with workspace_lock(self.root / "locked"):
            with self.assertRaisesRegex(ImportFailure, "busy"):
                with workspace_lock(self.root / "locked"):
                    pass
        with workspace_lock(self.root / "locked"):
            pass
        (self.workspace.root / "alias").symlink_to(self.root)
        with self.assertRaisesRegex(ImportFailure, "denied"):
            resolve_inside(self.workspace.root, "alias/secret")

    def test_post_json_and_form_download_templates(self):
        for body_key in ("json", "form"):
            config = copy.deepcopy(self.config)
            config["s3"]["request"] = {"url": self.base + "/post-s3", "method": "POST", body_key: {"id": "{s3uuid}"}}
            S3Downloader(self.http, config).download("image-1", self.root / body_key, 4096)
            self.assertEqual(self.server.fixture["post_body"], {"id": "image-1"})

    def test_local_inputs_and_cli_run(self):
        config_file = self.root / "config.json"
        input_file = self.root / "articles.json"
        save_json(config_file, self.config)
        save_json(input_file, self.server.fixture["articles"])
        self.assertEqual(len(list(local_articles(input_file))), 3)
        with contextlib.redirect_stdout(io.StringIO()) as output:
            code = main(["--config", str(config_file), "run", "--input", str(input_file), "--wait-seconds", "2"])
        self.assertEqual(code, 0, output.getvalue())
        self.assertNotIn("synthetic-import-key", output.getvalue())
        self.assertNotIn("合成数据库排障", output.getvalue())
        reads = self.server.fixture["reads"]
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(main(["--config", str(config_file), "status"]), 0)
        self.assertEqual(reads, self.server.fixture["reads"])

    def test_init_does_not_overwrite_config_and_placeholders_fail_clearly(self):
        config_file = self.root / "init.json"
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(main(["--config", str(config_file), "init"]), 0)
            before = config_file.read_bytes()
            self.assertEqual(main(["--config", str(config_file), "init"]), 1)
        self.assertEqual(config_file.read_bytes(), before)
        config = load_config(config_file)
        with self.assertRaisesRegex(ImportFailure, "endpoint_placeholder"):
            list(fetch_articles(config, Transport(config)))

    def test_https_and_environment_credentials_required(self):
        config = copy.deepcopy(self.config)
        config["http"]["allow_insecure_http"] = False
        with self.assertRaisesRegex(ImportFailure, "https_required"):
            list(fetch_articles(config, Transport(config)))
        with self.assertRaisesRegex(ImportFailure, "credentials_must_use_environment"):
            self.http.headers({"headers": {"Authorization": "secret"}})

    def test_image_attachment_is_wrapped_and_reuses_body_download(self):
        value = article(attachments=[{"s3uuid": "image-1", "fileExtension": "png"}])
        counts = self.prepare([value])
        self.assertEqual(counts["prepared"], 1)
        self.assertEqual(self.server.fixture["downloads"]["image-1"], 1)
        manifest = self.workspace.manifest(next(iter(self.workspace.state["articles"].values())))
        self.assertEqual(len(manifest["artifacts"]), 2)
        self.assertEqual(manifest["artifacts"][1]["source_extension"], "png")
        self.assertTrue(manifest["artifacts"][1]["path"].endswith(".html"))

    def test_refresh_redownloads_same_uuid_with_changed_bytes(self):
        self.prepare([article()])
        old = self.workspace.state["articles"][safe_name("A-100")]["manifest"]
        self.server.fixture["resources"]["attachment-1"] = b"synthetic replacement bytes"
        self.assertEqual(self.prepare([article()])["unchanged"], 1)
        self.assertEqual(self.prepare([article()], refresh=True)["prepared"], 1)
        self.assertNotEqual(self.workspace.state["articles"][safe_name("A-100")]["manifest"], old)

    def test_unknown_upload_is_not_blindly_repeated(self):
        self.prepare([article(attachments=[], content="<p>synthetic</p>")])
        publisher = Publisher(self.workspace, self.http)
        self.server.fixture["drop_upload_response"] = True
        publisher.upload()
        self.server.fixture["documents"].clear()
        self.assertEqual(publisher.upload()["failed"], 1)
        self.assertEqual(self.server.fixture["uploads"], 1)
        self.assertEqual(next(iter(publisher.records.values()))["error"]["code"], "mindcreek.upload_unresolved")

    def test_prune_revalidates_remote_id_and_filename(self):
        self.prepare([article(attachments=[], content="<p>old</p>")])
        publisher = Publisher(self.workspace, self.http)
        publisher.upload()
        old = next(iter(publisher.records.values()))
        self.prepare([article(attachments=[], content="<p>new</p>")])
        publisher.upload()
        self.server.fixture["documents"][old["id"]]["file_name"] = "unrelated-file.txt"
        with self.assertRaisesRegex(ImportFailure, "identity_mismatch"):
            publisher.prune(apply=True)
        self.assertFalse(self.server.fixture["deletes"])

    def test_duplicate_ids_and_bad_metadata_are_not_silently_accepted(self):
        counts = self.prepare([article(), article(content="<p>conflicting version</p>")])
        self.assertEqual(counts["failed"], 1)
        self.assertEqual(self.workspace.state["articles"][safe_name("A-100")]["error"]["code"], "article.duplicate_id_conflict")
        self.assertEqual(self.prepare([article(contentType=9)])["failed"], 1)

    def test_single_article_and_jsonl_inputs(self):
        single, jsonl = self.root / "one.json", self.root / "many.jsonl"
        save_json(single, article())
        jsonl.write_text(json.dumps(article()) + "\n\n" + json.dumps(article("A-200")) + "\n")
        self.assertEqual(len(list(local_articles(single))), 1)
        self.assertEqual(len(list(local_articles(jsonl))), 2)
        jsonl.write_text('{"private": broken secret}\n')
        with self.assertRaisesRegex(ImportFailure, "invalid_jsonl"):
            list(local_articles(jsonl))

    def test_cursor_offset_and_detail_adapter_shapes(self):
        config = copy.deepcopy(self.config)
        config["articles"]["pagination"] = {"mode": "cursor", "page_size": 1}
        with patch.object(self.http, "json_request", side_effect=[
            {"data": {"items": [article()], "nextCursor": "next"}},
            {"data": {"items": [article("A-200")], "nextCursor": None}},
        ]) as request:
            self.assertEqual(len(list(fetch_articles(config, self.http))), 2)
            self.assertEqual(request.call_count, 2)
        config["articles"]["pagination"] = {"mode": "offset", "page_size": 1}
        config["articles"]["request"]["query"] = {"offset": "{offset}", "limit": "{page_size}"}
        offsets = []

        def pages(spec, variables):
            offsets.append(variables["offset"])
            return {"data": {"items": [article()] if variables["offset"] == 0 else []}}

        with patch.object(self.http, "json_request", side_effect=pages):
            self.assertEqual(len(list(fetch_articles(config, self.http))), 1)
        self.assertEqual(offsets, [0, 1])
        config["articles"]["pagination"] = {"mode": "none"}
        config["articles"]["items_path"] = "$"
        config["articles"]["detail"] = {"request": {"url": self.base + "/detail?code={articleCode}"}, "item_path": "data"}
        with patch.object(self.http, "json_request", side_effect=[[{"articleCode": "A-100"}], {"data": article()}]):
            self.assertEqual(list(fetch_articles(config, self.http))[0]["aritcleTitle"], article()["aritcleTitle"])

    def test_error_output_does_not_include_credentials_or_body(self):
        config_file = self.root / "error-config.json"
        config = copy.deepcopy(self.config)
        config["articles"]["request"]["url"] = self.base + "/missing?token=private-should-not-appear"
        save_json(config_file, config)
        with contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(main(["--config", str(config_file), "fetch"]), 1)
        self.assertIn('"status": 404', output.getvalue())
        self.assertNotIn("private-should-not-appear", output.getvalue())
        self.assertNotIn("synthetic_not_found", output.getvalue())

    def test_check_does_not_upload_missing_files(self):
        self.prepare([article()])
        result = Publisher(self.workspace, self.http).upload(check_only=True)
        self.assertEqual(result["not_uploaded"], 2)
        self.assertEqual(self.server.fixture["uploads"], 0)

    def test_downloaded_html_main_and_markdown_attachment_embed_images(self):
        self.server.fixture["resources"]["html-main"] = b'<p>synthetic</p><img src="image-1">'
        self.server.fixture["resources"]["md-attachment"] = b"![synthetic](image-1)"
        value = article(kind=2, contentFileUuid="html-main", contentFileExtension="html",
                        attachments=[{"s3uuid": "md-attachment", "fileExtension": "md"}])
        self.assertEqual(self.prepare([value])["prepared"], 1)
        entry = next(iter(self.workspace.state["articles"].values()))
        manifest = self.workspace.manifest(entry)
        self.assertEqual(len(manifest["originals"]), 2)
        self.assertEqual(len(manifest["images"]), 1)
        self.assertEqual(self.server.fixture["downloads"]["image-1"], 1)
        directory = resolve_inside(self.workspace.root, entry["manifest"]).parent
        for artifact in manifest["artifacts"][1:]:
            self.assertIn("data:image/png;base64,", (directory / artifact["path"]).read_text())

    def test_sparse_page_with_has_more_does_not_truncate_source(self):
        config = copy.deepcopy(self.config)
        config["articles"]["pagination"]["has_more_path"] = "data.hasMore"
        with patch.object(self.http, "json_request", side_effect=[
            {"data": {"items": [], "hasMore": True}},
            {"data": {"items": [article()], "hasMore": False}},
        ]):
            self.assertEqual(len(list(fetch_articles(config, self.http))), 1)

    def test_incomplete_local_snapshot_blocks_prune(self):
        self.prepare([article()])

        def interrupted():
            yield article()
            raise ImportFailure("synthetic.interrupted_input")

        with self.assertRaisesRegex(ImportFailure, "interrupted_input"):
            self.workspace.prepare(interrupted(), self.downloader)
        self.assertTrue(self.workspace.state["prepare_incomplete"])
        with self.assertRaisesRegex(ImportFailure, "article_failure_blocks"):
            Publisher(self.workspace, self.http).prune()

    def test_offline_example_never_calls_s3(self):
        example = Path(__file__).resolve().parents[1] / "examples/offline-article.json"
        with patch.object(self.downloader, "download", side_effect=AssertionError("unexpected request")):
            self.assertEqual(self.workspace.prepare(local_articles(example), self.downloader)["prepared"], 1)

    def test_network_failure_classification_without_private_detail(self):
        import ssl
        import urllib.error

        cases = [(ssl.SSLCertVerificationError("private-corporate-name"), "http.tls_verification_failed"),
                 (TimeoutError("private-token"), "http.timeout"),
                 (socket.gaierror("private-host"), "http.dns_failed"),
                 (ConnectionRefusedError("private-host"), "http.connection_refused")]
        for reason, expected in cases:
            with patch.object(self.http.opener, "open", side_effect=urllib.error.URLError(reason)):
                with self.assertRaises(ImportFailure) as error:
                    self.http.json_request({"url": self.base + "/articles"}, {})
            self.assertEqual(error.exception.code, expected)
            self.assertNotIn("private", str(error.exception))

    def test_conflicting_batch_preserves_last_usable_revision(self):
        self.prepare([article()])
        previous = dict(self.workspace.state["articles"][safe_name("A-100")])
        counts = self.prepare([article(content="<p>first new version</p>"),
                               article(content="<p>conflicting new version</p>"), article("A-200")])
        self.assertEqual(counts["failed"], 1)
        self.assertEqual(counts["prepared"], 1)  # Only the independent article is accepted.
        entry = self.workspace.state["articles"][safe_name("A-100")]
        self.assertEqual(entry["manifest"], previous["manifest"])
        self.assertEqual(entry["fingerprint"], previous["fingerprint"])
        self.assertEqual(entry["error"]["code"], "article.duplicate_id_conflict")
        self.assertIn(safe_name("A-200"), self.workspace.state["articles"])
        self.assertEqual(self.prepare([article()])["unchanged"], 1)
        self.assertNotIn("error", entry)

    def test_markdown_literal_images_and_mixed_fences_are_not_downloaded(self):
        content = ('\\![escaped](not-real)\n``literal ` ![code](not-real)``\n'
                   '````md\n```\n![nested](not-real)\n~~~\n![still-code](not-real)\n````\n'
                   '~~~md\n```\n![other](not-real)\n~~~\n'
                   '- List item\n    ![nested-list-image](image-1)\n\n![actual](image-1)\n')
        value = markdown(content, Images(self.root / "literal-markdown", self.downloader, {}))
        self.assertEqual(value.count("data:image/png;base64,"), 2)
        self.assertEqual(value.count("not-real"), content.count("not-real"))
        self.assertEqual(self.server.fixture["downloads"], {"image-1": 1})

    def test_malformed_configuration_returns_sanitized_error(self):
        config_file = self.root / "invalid-config.json"
        cases = [("http", None), ("limits", []), ("articles", {"request": []}),
                 ("articles", {"request": {"method": {"private-token": "do-not-print"}}}),
                 ("articles", {"pagination": []}), ("articles", {"items_path": []}),
                 ("s3", {"download_origins": "private-do-not-print"}),
                 ("mindcreek", {"base_url": []})]
        for section, value in cases:
            with self.subTest(section=section, value=value):
                config = copy.deepcopy(self.config)
                config[section] = value
                save_json(config_file, config)
                with contextlib.redirect_stdout(io.StringIO()) as output:
                    self.assertEqual(main(["--config", str(config_file), "fetch"]), 1)
                self.assertIn('"code": "config.', output.getvalue())
                self.assertNotIn("do-not-print", output.getvalue())
        self.assertEqual(self.server.fixture["reads"], 0)

    def test_status_reports_parsing_state_and_only_current_destination(self):
        config_file = self.root / "status-config.json"
        root = Path(self.config["output_dir"]) / safe_name(self.config["source_id"])
        workspace = Workspace(root, self.config)
        workspace.prepare([article(attachments=[], content="<p>status</p>")], self.downloader)
        save_json(config_file, self.config)

        def status(expected, field):
            reads = self.server.fixture["reads"]
            with contextlib.redirect_stdout(io.StringIO()) as output:
                self.assertEqual(main(["--config", str(config_file), "status"]), expected)
            self.assertEqual(json.loads(output.getvalue())["uploads"][field], 1)
            self.assertEqual(self.server.fixture["reads"], reads)

        status(2, "not_uploaded")
        publisher = Publisher(workspace, self.http)
        self.server.fixture["fail_parse"] = True
        publisher.upload()
        status(1, "failed")
        record = next(iter(publisher.records.values()))
        record["parse_status"] = "pending"
        workspace.save()
        status(2, "pending")
        self.server.fixture["fail_parse"] = False
        publisher.upload(retry_failed=True)
        status(0, "completed")
        changed = copy.deepcopy(self.config)
        changed["mindcreek"]["knowledge_base_id"] = "kb-another"
        save_json(config_file, changed)
        status(2, "not_uploaded")

    def test_cli_staged_pipeline_across_process_restarts(self):
        entry = Path(__file__).resolve().parents[1] / "import_posts.py"
        config_file = self.root / "process-config.json"
        save_json(config_file, self.config)

        def invoke(*args, expected=0):
            result = subprocess.run([sys.executable, str(entry), "--config", str(config_file), *args],
                                    text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
            self.assertNotIn("synthetic-import-key", result.stdout + result.stderr)
            return json.loads(result.stdout.splitlines()[-1])

        self.assertEqual(invoke("fetch")["articles"], 3)
        self.assertEqual(invoke("prepare")["prepared"], 3)
        self.assertEqual(invoke("status", expected=2)["uploads"]["not_uploaded"], 5)
        self.assertEqual(invoke("check", expected=2)["not_uploaded"], 5)
        self.assertEqual(self.server.fixture["uploads"], 0)
        self.assertEqual(invoke("upload", "--wait-seconds", "2")["completed"], 5)
        self.assertEqual(invoke("check")["completed"], 5)
        self.assertEqual(invoke("status")["uploads"]["completed"], 5)
        self.assertEqual(invoke("run")["completed"], 5)
        self.assertEqual(self.server.fixture["uploads"], 5)
        self.assertEqual(invoke("prune")["candidates"], [])
        self.assertFalse(self.server.fixture["deletes"])


if __name__ == "__main__":
    unittest.main()
