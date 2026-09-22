"""Use native KB/document APIs through the gateway; reconcile uncertain writes."""

from __future__ import annotations

import json
import time
import urllib.parse
import urllib.request
import uuid
from pathlib import Path
from typing import Any

from .common import ImportFailure, canonical, digest
from .transport import Transport
from .workspace import Workspace


def destination_key(config: dict[str, Any]) -> str:
    target = config.get("mindcreek", {})
    return digest(canonical([target.get("base_url", "").rstrip("/"), target.get("knowledge_base_id", "")]))


def create_knowledge_base(workspace: Workspace, http: Transport, name: str) -> dict[str, Any]:
    """Explicit native create, with durable intent and reconciliation after interruption."""
    config = workspace.config.get("mindcreek", {})
    base = config.get("base_url", "").rstrip("/")
    http.validate_url(base)
    if urllib.parse.urlsplit(base).query or not name.strip() or len(name) > 100:
        raise ImportFailure("config.knowledge_base_name_invalid")
    headers = http.headers(config)
    if not any(k.lower() in {"authorization", "x-api-key"} for k in headers):
        raise ImportFailure("config.mindcreek_authentication_required")
    tenant = next((v for k, v in headers.items() if k.lower() == "x-tenant-id"), "")
    identity = digest(canonical([base, tenant, name]))
    record = workspace.state.setdefault("knowledge_base_creations", {}).setdefault(identity, {})
    marker = "MindCreek community import " + workspace.state["installation_id"] + ":" + identity
    endpoint = base + "/api/v1/knowledge-bases"

    def request(method, suffix="", body=None):
        req = urllib.request.Request(endpoint + suffix, method=method, data=None if body is None else canonical(body),
                                     headers={**headers, "Content-Type":"application/json"})
        raw, _ = http.transfer(req, 16 * 1024 * 1024, retry_read=method == "GET")
        try:
            value = json.loads(raw)
        except ValueError as exc:
            raise ImportFailure("mindcreek.invalid_json") from exc
        if not isinstance(value, dict) or value.get("success") is not True:
            raise ImportFailure("mindcreek.unsuccessful_response")
        return value.get("data")

    if record.get("id"):
        value = request("GET", "/" + urllib.parse.quote(record["id"], safe=""))
        if not isinstance(value, dict) or value.get("id") != record["id"]:
            raise ImportFailure("mindcreek.knowledge_base_identity_mismatch")
        return {"knowledge_base_id":record["id"], "created":False}
    if record.get("attempted"):
        values = request("GET")
        if not isinstance(values, list):
            raise ImportFailure("mindcreek.invalid_list")
        matches = [v for v in values if v.get("name") == name and v.get("description") == marker]
        if len(matches) != 1 or not matches[0].get("id"):
            raise ImportFailure("mindcreek.knowledge_base_create_unresolved")
        value = matches[0]
        created = False
    else:
        record["attempted"] = True
        workspace.save()
        try:
            value = request("POST", body={"name":name,"description":marker,"type":"document"})
        except ImportFailure as exc:
            if exc.status in (400,401,403,404,405,413,415,422,429):
                record["attempted"] = False
                workspace.save()
            raise
        created = True
    if not isinstance(value, dict) or not isinstance(value.get("id"), str) or not value["id"]:
        raise ImportFailure("mindcreek.knowledge_base_identity_mismatch")
    record["id"] = value["id"]
    workspace.save()
    return {"knowledge_base_id":value["id"],"created":created}


def upload_counts(workspace: Workspace, desired: dict, records: dict) -> dict[str, int]:
    article_failures = max(sum(bool(item.get("error")) for item in workspace.state["articles"].values()),
                           workspace.state.get("last_prepare", {}).get("failed", 0),
                           int(bool(workspace.state.get("prepare_incomplete"))))
    result = {"completed": 0, "pending": 0, "failed": 0, "not_uploaded": 0, "article_failures": article_failures}
    for key in desired:
        record = records.get(key)
        if record and (record.get("error") or record.get("deleted") or record.get("parse_status") in ("failed", "cancelled", "deleting")):
            result["failed"] += 1
        elif not record or (not record.get("id") and not record.get("attempted")):
            result["not_uploaded"] += 1
        elif record.get("parse_status") == "completed":
            result["completed"] += 1
        else:
            result["pending"] += 1
    return result


class Multipart:
    def __init__(self, path: Path, filename: str):
        self.path = path
        self.boundary = "mindcreek-" + uuid.uuid4().hex
        self.head = (f'--{self.boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\n'
                     'Content-Type: application/octet-stream\r\n\r\n').encode()
        self.tail = f"\r\n--{self.boundary}--\r\n".encode()
        self.length = len(self.head) + path.stat().st_size + len(self.tail)

    def __iter__(self):
        yield self.head
        with self.path.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                yield chunk
        yield self.tail


class Publisher:
    def __init__(self, workspace: Workspace, http: Transport):
        self.workspace, self.http = workspace, http
        config = workspace.config.get("mindcreek", {})
        self.base = config.get("base_url", "").rstrip("/")
        self.kb = config.get("knowledge_base_id", "")
        http.validate_url(self.base)
        if not isinstance(self.kb, str) or not self.kb or "REPLACE_" in self.kb:
            raise ImportFailure("config.knowledge_base_id_required")
        if urllib.parse.urlsplit(self.base).query:
            raise ImportFailure("config.base_url_query_denied")
        self.headers = http.headers(config)
        if not any(name.lower() in {"authorization", "x-api-key"} for name in self.headers):
            raise ImportFailure("config.mindcreek_authentication_required")
        self.endpoint = self.base + "/api/v1/knowledge-bases/" + urllib.parse.quote(self.kb, safe="") + "/knowledge"
        self.records = workspace.state["uploads"].setdefault(destination_key(workspace.config), {})
        self.workspace.save()

    def request(self, method: str, suffix: str = "", data=None, headers=None) -> dict[str, Any]:
        if suffix.startswith("/"):
            target = self.base + "/api/v1/knowledge" + suffix
        elif method == "POST" and not suffix:
            target = self.endpoint + "/file"
        else:
            target = self.endpoint + suffix
        request = urllib.request.Request(target, data=data, method=method,
                                         headers={**self.headers, **(headers or {})})
        raw, _ = self.http.transfer(request, 16 * 1024 * 1024, retry_read=method == "GET")
        try:
            response = json.loads(raw)
        except (ValueError, UnicodeError) as exc:
            raise ImportFailure("mindcreek.invalid_json") from exc
        if not isinstance(response, dict) or response.get("success") is not True:
            raise ImportFailure("mindcreek.unsuccessful_response")
        return response

    def validate_document(self, value: Any, record: dict[str, Any]) -> dict[str, Any]:
        if (not isinstance(value, dict) or not isinstance(value.get("id"), str) or not value["id"]
                or value.get("knowledge_base_id") != self.kb or value.get("file_name") != record["filename"]
                or value.get("file_size") != record["bytes"]):
            raise ImportFailure("mindcreek.document_identity_mismatch")
        if record.get("id") and record["id"] != value["id"]:
            raise ImportFailure("mindcreek.document_identity_mismatch")
        return value

    def refresh(self, record: dict[str, Any]) -> None:
        suffix = "/" + urllib.parse.quote(record["id"], safe="")
        value = self.validate_document(self.request("GET", suffix).get("data"), record)
        status = value.get("parse_status")
        if not isinstance(status, str):
            raise ImportFailure("mindcreek.parse_status_missing")
        record["parse_status"] = status
        record.pop("error", None)
        self.workspace.save()

    def reconcile(self, record: dict[str, Any]) -> bool:
        """Match our persisted, installation-specific filename after an interrupted POST."""
        matches, seen = [], set()
        for page in range(1, 10001):
            items = self.request("GET", f"?page={page}&page_size=100").get("data")
            if not isinstance(items, list):
                raise ImportFailure("mindcreek.invalid_list")
            for item in items:
                if not isinstance(item, dict) or not item.get("id") or item["id"] in seen:
                    raise ImportFailure("mindcreek.unstable_pagination")
                seen.add(item["id"])
                if item.get("file_name") == record["filename"]:
                    matches.append(self.validate_document(item, record))
            if len(items) < 100:
                break
        else:
            raise ImportFailure("mindcreek.page_limit")
        if len(matches) > 1:
            raise ImportFailure("mindcreek.ambiguous_upload")
        if matches:
            record.update({"id": matches[0]["id"], "parse_status": matches[0].get("parse_status", "pending"),
                           "owned": False, "reconciled": True})
            record.pop("error", None)
            self.workspace.save()
            return True
        # No automatic repeat of an uncertain non-idempotent write: eventual visibility
        # or a late server completion could otherwise create duplicates.
        raise ImportFailure("mindcreek.upload_unresolved")

    def upload(self, *, check_only: bool = False, wait_seconds: float = 0, retry_failed: bool = False) -> dict[str, int]:
        desired = self.workspace.desired()
        # Always check current authorization, even when every local hash is cached.
        self.request("GET", "?page=1&page_size=1")
        for key, item in desired.items():
            record = self.records.get(key)
            if record is None and check_only:
                continue
            if record is None:
                filename = item["filename"]
                record = {"filename": filename, "bytes": item["bytes"], "articles": item["articles"],
                          "owned": False, "attempted": False, "parse_status": "not_uploaded"}
                self.records[key] = record
            record["articles"] = item["articles"]
            try:
                if record.get("deleted"):
                    raise ImportFailure("mindcreek.previously_retired_content")
                if record.get("id"):
                    self.refresh(record)
                    if retry_failed and record["parse_status"] in ("failed", "cancelled"):
                        suffix = "/" + urllib.parse.quote(record["id"], safe="") + "/reparse"
                        self.request("POST", suffix)
                        self.refresh(record)
                elif record.get("attempted"):
                    self.reconcile(record)
                elif not check_only:
                    body = Multipart(Path(item["absolute_path"]), record["filename"])
                    record["attempted"] = True
                    record["parse_status"] = "pending"
                    self.workspace.save()  # Persist intent before a non-idempotent HTTP write.
                    value = self.validate_document(self.request("POST", data=body, headers={
                        "Content-Type": "multipart/form-data; boundary=" + body.boundary,
                        "Content-Length": str(body.length),
                    }).get("data"), record)
                    record.update({"id": value["id"], "owned": True, "parse_status": value.get("parse_status", "pending")})
                record.pop("error", None)
            except ImportFailure as exc:
                record["error"] = {"code": exc.code, "status": exc.status}
                if not record.get("id") and exc.status in (400, 401, 403, 404, 405, 413, 415, 422, 429):
                    record["attempted"] = False  # Definitive rejection, not an uncertain accepted upload.
            self.workspace.save()
        deadline = time.monotonic() + wait_seconds
        while True:
            for key in desired:
                record = self.records.get(key, {})
                if record.get("id") and not record.get("error") and record.get("parse_status") not in ("completed", "failed", "cancelled"):
                    try:
                        self.refresh(record)
                    except ImportFailure as exc:
                        record["error"] = {"code": exc.code, "status": exc.status}
                        self.workspace.save()
            result = upload_counts(self.workspace, desired, self.records)
            if not result["pending"] or time.monotonic() >= deadline:
                return result
            time.sleep(min(2, max(0, deadline - time.monotonic())))

    def prune(self, *, apply: bool = False) -> dict[str, Any]:
        """Only explicitly confirmed retirement of our obsolete, verified remote IDs."""
        if (self.workspace.state.get("prepare_incomplete") or self.workspace.state.get("last_prepare", {}).get("failed")
                or any(item.get("error") for item in self.workspace.state["articles"].values())):
            raise ImportFailure("prune.article_failure_blocks_retirement")
        desired = self.workspace.desired(include_failed=True)
        if not desired:
            raise ImportFailure("prune.empty_snapshot_denied")
        self.request("GET", "?page=1&page_size=1")
        for key in desired:
            record = self.records.get(key)
            if not record or not record.get("id") or record.get("deleted"):
                raise ImportFailure("prune.replacement_not_ready")
            self.refresh(record)
            if record.get("parse_status") != "completed":
                raise ImportFailure("prune.replacement_not_ready")
        candidates = []
        for key, record in self.records.items():
            if key not in desired and record.get("owned") and record.get("id") and not record.get("deleted"):
                self.refresh(record)
                candidates.append(record)
        result = {"candidates": [item["id"] for item in candidates], "deletion_requested": 0, "applied": apply}
        if apply:
            for item in candidates:
                self.request("DELETE", "/" + urllib.parse.quote(item["id"], safe=""))
                item["deleted"] = True
                self.workspace.save()
                result["deletion_requested"] += 1
        return result
