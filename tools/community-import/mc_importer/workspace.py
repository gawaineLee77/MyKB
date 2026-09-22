"""Versioned, per-article local exports; never destroy the previous usable revision."""

from __future__ import annotations

import os
import html
import shutil
import tempfile
import uuid
from pathlib import Path
from typing import Any, Iterable

from .common import (ALLOWED_DOCUMENTS, ImportFailure, atomic_bytes, canonical, digest, extension,
                     file_hash, private_dir, read_json, resolve_inside, safe_name, save_json, upload_name, validate_article)
from .content import Images, markdown, metadata_html, rich_html
from .transport import S3Downloader


def document_signature(path: Path, ext: str) -> None:
    with path.open("rb") as handle:
        head = handle.read(1024)
    if not head:
        raise ImportFailure("document.empty")
    if ext == "pdf" and b"%PDF-" not in head:
        raise ImportFailure("document.signature_mismatch")
    if ext in {"docx", "xlsx", "pptx"} and not head.startswith(b"PK\x03\x04"):
        raise ImportFailure("document.signature_mismatch")
    if ext in {"doc", "xls", "ppt"} and not head.startswith(b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"):
        raise ImportFailure("document.signature_mismatch")


class Workspace:
    def __init__(self, root: Path, config: dict[str, Any]):
        self.root, self.config = private_dir(root), config
        self.state_path = resolve_inside(root, "state.json")
        if self.state_path.exists():
            self.state = read_json(self.state_path)
            if not isinstance(self.state, dict) or self.state.get("schema_version") != 1 or self.state.get("source_id") != config["source_id"]:
                raise ImportFailure("storage.state_mismatch")
        else:
            self.state = {"schema_version": 1, "source_id": config["source_id"], "installation_id": uuid.uuid4().hex,
                          "articles": {}, "uploads": {}}
            self.save()

    def save(self) -> None:
        save_json(self.state_path, self.state)

    def manifest(self, entry: dict[str, Any], *, verify: bool = True) -> dict[str, Any]:
        path = resolve_inside(self.root, entry["manifest"])
        manifest = read_json(path)
        if verify:
            for item in manifest["artifacts"] + manifest["images"] + manifest.get("archived_attachments", []) + manifest.get("originals", []):
                file = resolve_inside(path.parent, item["path"])
                if not file.is_file() or file_hash(file) != item["sha256"]:
                    raise ImportFailure("storage.artifact_changed")
        return manifest

    def prepare(self, articles: Iterable[dict[str, Any]], downloader: S3Downloader, *, refresh: bool = False) -> dict[str, int]:
        counts = {"prepared": 0, "unchanged": 0, "failed": 0, "warnings": 0}
        self.state["prepare_incomplete"] = True
        self.save()
        seen, previous_entries, accepted = {}, {}, {}
        for index, raw in enumerate(articles):
            code = raw.get("articleCode", "") if isinstance(raw, dict) else ""
            if type(code) is int:
                code = str(code)
            key = safe_name(code) if isinstance(code, str) and code else f"invalid-{index}"
            entry = self.state["articles"].setdefault(key, {}) if isinstance(code, str) and code else {}
            previous_entries.setdefault(key, dict(entry))
            try:
                article = validate_article(raw)
                fingerprint = digest(canonical(article))
                if key in seen:
                    if seen[key] != fingerprint:
                        # A conflicting batch has no authoritative revision. Keep
                        # the last usable pointer from before this preparation.
                        entry.clear()
                        entry.update(previous_entries[key])
                        if key in accepted:
                            outcome, warnings = accepted.pop(key)
                            counts[outcome] -= 1
                            counts["warnings"] -= warnings
                        raise ImportFailure("article.duplicate_id_conflict")
                    continue
                seen[key] = fingerprint
                if not refresh and entry.get("fingerprint") == fingerprint and entry.get("manifest"):
                    self.manifest(entry)
                    entry.pop("error", None)
                    counts["unchanged"] += 1
                    outcome = "unchanged"
                else:
                    manifest = self.build(article, downloader)
                    entry.update({"manifest": manifest, "fingerprint": fingerprint})
                    entry.pop("error", None)
                    counts["prepared"] += 1
                    outcome = "prepared"
                warnings = len(self.manifest(entry, verify=False)["warnings"])
                counts["warnings"] += warnings
                accepted[key] = outcome, warnings
            except ImportFailure as exc:
                entry["error"] = {"code": exc.code, "status": exc.status}
                counts["failed"] += 1
            self.save()
        self.state["last_prepare"] = counts
        self.state["prepare_incomplete"] = False
        self.save()
        return counts

    def build(self, article: dict[str, Any], downloader: S3Downloader) -> str:
        key = safe_name(article["articleCode"])
        parent = resolve_inside(self.root, f"articles/{key}/versions")
        private_dir(parent)
        stage = Path(tempfile.mkdtemp(prefix=".prepare-", dir=parent))
        limits = self.config.get("limits", {})
        max_file = int(limits.get("max_file_mib", 500)) * 1024 * 1024
        try:
            if len(article["attachments"]) > int(limits.get("max_attachments_per_article", 100)):
                raise ImportFailure("attachment.count_limit")
            artifacts, archived, attachments, warnings, originals = [], [], [], [], []
            downloaded: dict[str, dict[str, Any]] = {}
            images = Images(stage, downloader, limits)
            private_dir(stage / "body")
            private_dir(stage / "attachments")

            def record(path: Path, role: str, file_uuid: str = "") -> dict[str, Any]:
                size = path.stat().st_size
                if not 0 < size <= max_file:
                    raise ImportFailure("document.size_limit")
                sha = file_hash(path)
                # Shared binary resources get the same name even when different posts reference them.
                title = "资料" if file_uuid else article["aritcleTitle"]
                name = upload_name(self.state["installation_id"], sha + path.suffix.lower(), title)
                value = {"path": str(path.relative_to(stage)), "filename": name, "role": role,
                         "sha256": sha, "bytes": size, "s3uuid": file_uuid}
                return value

            def download(file_uuid: str, ext: str, role: str) -> dict[str, Any]:
                if file_uuid in downloaded:
                    previous = downloaded[file_uuid]
                    if previous.get("source_extension") != ext:
                        raise ImportFailure("article.resource_extension_conflict")
                    return previous
                if ext in {"png", "jpg", "jpeg", "gif", "webp"}:
                    image = images.embed(file_uuid)
                    path = stage / "attachments" / (safe_name(file_uuid) + "-image.html")
                    wrapper = ('<!doctype html><html><head><meta charset="utf-8"></head><body>'
                               + metadata_html(article, []) + '<h2>图片资料</h2><img src="' + image
                               + '" alt="' + html.escape(file_uuid, quote=True) + '"></body></html>')
                    atomic_bytes(path, wrapper.encode())
                    value = {**record(path, role, file_uuid), "source_extension": ext}
                    artifacts.append(value)
                    downloaded[file_uuid] = value
                    return value
                supported = ext in ALLOWED_DOCUMENTS
                if not supported and (role == "main" or self.config.get("unsupported_attachments", "error") != "archive"):
                    raise ImportFailure("document.extension_unsupported")
                directory = "body" if role == "main" else "attachments"
                path = stage / directory / f"{key}-{safe_name(file_uuid)}.{ext}"
                downloader.download(file_uuid, path, max_file)
                if supported:
                    document_signature(path, ext)
                if supported and ext in ("md", "html", "htm"):
                    original_path = stage / "originals" / path.name
                    private_dir(original_path.parent)
                    shutil.copyfile(path, original_path)
                    os.chmod(original_path, 0o600)
                    originals.append({"path": str(original_path.relative_to(stage)), "sha256": file_hash(original_path),
                                      "bytes": original_path.stat().st_size})
                    try:
                        content = path.read_text(encoding="utf-8-sig")
                    except UnicodeError as exc:
                        raise ImportFailure("document.utf8_required_for_image_normalization") from exc
                    if ext == "md":
                        normalized = markdown(content, images)
                    else:
                        normalized = ('<!doctype html><html><head><meta charset="utf-8"></head><body>'
                                      + rich_html(content, images) + '</body></html>')
                    atomic_bytes(path, normalized.encode("utf-8"))
                value = {**record(path, role, file_uuid), "source_extension": ext}
                downloaded[file_uuid] = value
                if supported:
                    artifacts.append(value)
                else:
                    archived.append(value)
                    warnings.append({"code": "attachment.archived_not_indexed", "resource": safe_name(file_uuid)})
                return value

            main = None
            if article["contentType"] == 2:
                main = download(article["contentFileUuid"], extension(article["contentFileExtension"]), "main")
            for item in article["attachments"]:
                attachments.append(download(item["s3uuid"], extension(item["fileExtension"]), "attachment"))
            metadata = metadata_html(article, attachments)
            if article["contentType"] == 3:
                body = metadata + "\n\n---\n\n" + markdown(article["content"], images)
                path = stage / "body" / (key + "-post.md")
            else:
                content = rich_html(article["content"], images) if article["contentType"] == 1 else (
                    "<p>主文档独立导入，文件名：" + main["filename"] + "</p>")
                body = '<!doctype html><html><head><meta charset="utf-8"></head><body>' + metadata + "<hr>" + content + "</body></html>"
                path = stage / "body" / (key + ("-index.html" if main else "-post.html"))
            if len(body.encode("utf-8")) > max_file:
                raise ImportFailure("document.size_limit")
            atomic_bytes(path, body.encode("utf-8"))
            artifacts.insert(0, record(path, "index" if main else "main"))
            manifest = {"schema_version": 1, "source_id": self.config["source_id"], "articleCode": article["articleCode"],
                        "aritcleTitle": article["aritcleTitle"], "articleAbstract": article.get("articleAbstract"),
                        "contentType": article["contentType"], "lastUpdateDate": article.get("lastUpdateDate"),
                        "kmsUrl": article.get("kmsUrl"), "artifacts": artifacts, "images": images.records,
                        "archived_attachments": archived, "originals": originals, "warnings": warnings}
            save_json(stage / "manifest.json", manifest)
            # Keep normalized source fields only: unexpected API fields may contain credentials.
            source_keys = ("articleCode", "aritcleTitle", "articleAbstract", "content", "contentType", "attachments",
                           "contentFileUuid", "contentFileExtension", "lastUpdateDate", "kmsUrl")
            original = {key: article[key] for key in source_keys if key in article}
            original["attachments"] = [{"s3uuid": item["s3uuid"], "fileExtension": item["fileExtension"]}
                                       for item in article["attachments"]]
            save_json(stage / "source.json", original)
            version = uuid.uuid4().hex
            target = parent / version
            os.replace(stage, target)
            return str((target / "manifest.json").relative_to(self.root))
        finally:
            if stage.exists():
                shutil.rmtree(stage)

    def desired(self, *, include_failed: bool = False) -> dict[str, dict[str, Any]]:
        result = {}
        for article_key, entry in self.state["articles"].items():
            if not entry.get("manifest") or (entry.get("error") and not include_failed):
                continue
            manifest = self.manifest(entry)
            base = resolve_inside(self.root, entry["manifest"]).parent
            for item in manifest["artifacts"]:
                key = item["sha256"] + Path(item["path"]).suffix.lower()
                value = result.setdefault(key, {**item, "absolute_path": str(resolve_inside(base, item["path"])), "articles": []})
                value["articles"].append(article_key)
        return result
