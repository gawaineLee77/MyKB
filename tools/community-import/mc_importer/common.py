"""Protected local state, deterministic identities, and configuration utilities."""

from __future__ import annotations

import contextlib
import hashlib
import json
import math
import os
import re
import tempfile
from pathlib import Path
from typing import Any


class ImportFailure(Exception):
    """Only a fixed, non-sensitive error code may be printed by the CLI."""

    def __init__(self, code: str, status: int = 0):
        self.code, self.status = code, status
        super().__init__(code)


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def file_hash(path: Path) -> str:
    result = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def safe_name(value: str) -> str:
    prefix = re.sub(r"[^A-Za-z0-9_-]", "-", value)[:40].strip("-") or "item"
    return prefix + "-" + digest(value.encode())[:16]


def upload_name(installation: str, key: str, title: str) -> str:
    prefix = re.sub(r"[^\w.-]", "-", title, flags=re.UNICODE).strip(".-")[:32] or "document"
    return f"{prefix}--mc-{installation[:12]}-{key}"


def private_dir(path: Path) -> Path:
    if path.is_symlink():
        raise ImportFailure("storage.symlink_denied")
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(path, 0o700)
    return path


def atomic_bytes(path: Path, content: bytes) -> None:
    private_dir(path.parent)
    if path.is_symlink():
        raise ImportFailure("storage.symlink_denied")
    descriptor, temporary = tempfile.mkstemp(prefix=".write-", dir=path.parent)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(content)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def save_json(path: Path, value: Any) -> None:
    atomic_bytes(path, json.dumps(value, ensure_ascii=False, indent=2).encode("utf-8") + b"\n")


def read_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError, UnicodeError) as exc:
        raise ImportFailure("input.invalid_json") from exc


def select(value: Any, selector: str) -> Any:
    if selector in ("", "$"):
        return value
    try:
        for part in selector.removeprefix("$.").split("."):
            value = value[int(part)] if isinstance(value, list) else value[part]
        return value
    except (KeyError, IndexError, TypeError, ValueError) as exc:
        raise ImportFailure("response.selector_missing") from exc


def substitute(value: Any, variables: dict[str, Any]) -> Any:
    """Expand named runtime placeholders, not shell commands or environment values."""
    if isinstance(value, dict):
        return {key: substitute(item, variables) for key, item in value.items()}
    if isinstance(value, list):
        return [substitute(item, variables) for item in value]
    if not isinstance(value, str):
        return value
    if value.startswith("{") and value.endswith("}") and value[1:-1] in variables:
        return variables[value[1:-1]]
    for name, replacement in variables.items():
        value = value.replace("{" + name + "}", str(replacement))
    return value


def load_config(path: Path) -> dict[str, Any]:
    config = read_json(path)
    if not isinstance(config, dict) or config.get("schema_version") != 1:
        raise ImportFailure("config.version_invalid")
    if not isinstance(config.get("source_id"), str) or not config["source_id"].strip():
        raise ImportFailure("config.source_id_required")
    for section in ("articles", "s3", "mindcreek", "http", "limits"):
        if not isinstance(config.get(section, {}), dict):
            raise ImportFailure("config.section_invalid")
    articles = config.get("articles", {})
    for section in ("pagination", "detail"):
        if section in articles and not isinstance(articles[section], dict):
            raise ImportFailure("config.section_invalid")
    requests = [articles, config.get("s3", {}), articles.get("detail", {})]
    for section in requests:
        if "request" in section and not isinstance(section["request"], dict):
            raise ImportFailure("config.request_invalid")
    for request in [section.get("request", {}) for section in requests] + [config.get("mindcreek", {})]:
        for key in ("url", "method", "base_url", "knowledge_base_id"):
            if key in request and not isinstance(request[key], str):
                raise ImportFailure("config.request_invalid")
        for key in ("headers", "headers_env", "query", "form"):
            if key in request and not isinstance(request[key], dict):
                raise ImportFailure("config.request_invalid")
        if any(not isinstance(value, str) for value in request.get("headers_env", {}).values()):
            raise ImportFailure("config.request_invalid")
    for section, keys in ((articles, ("items_path",)), (articles.get("detail", {}), ("item_path",)),
                          (articles.get("pagination", {}), ("mode", "next_cursor_path", "has_more_path")),
                          (config.get("s3", {}), ("response_mode", "url_path"))):
        if any(key in section and not isinstance(section[key], str) for key in keys):
            raise ImportFailure("config.selector_invalid")
    origins = config.get("s3", {}).get("download_origins", [])
    if not isinstance(origins, list) or any(not isinstance(value, str) for value in origins):
        raise ImportFailure("config.download_origins_invalid")
    if config.get("unsupported_attachments", "error") not in ("error", "archive"):
        raise ImportFailure("config.unsupported_attachment_policy_invalid")
    limits = config.get("limits", {})
    for name, default, ceiling in (("max_file_mib", 500, 500), ("max_image_mib", 20, 100),
                                   ("max_embedded_images_mib", 100, 300), ("max_images_per_article", 200, 10000),
                                   ("max_attachments_per_article", 100, 10000)):
        value = limits.get(name, default)
        if not isinstance(value, int) or isinstance(value, bool) or not 1 <= value <= ceiling:
            raise ImportFailure("config.resource_limit_invalid")
    return config


def resolve_inside(root: Path, relative: str) -> Path:
    if not isinstance(relative, str) or Path(relative).is_absolute():
        raise ImportFailure("storage.path_denied")
    candidate = root / relative
    if not candidate.resolve().is_relative_to(root.resolve()):
        raise ImportFailure("storage.path_denied")
    for item in (candidate, *candidate.parents):
        if item == root.parent:
            break
        if item.is_symlink():
            raise ImportFailure("storage.symlink_denied")
    return candidate


@contextlib.contextmanager
def workspace_lock(root: Path):
    """POSIX flock releases on crashes; no stale lock deletion or unsafe unlock flag."""
    import fcntl

    private_dir(root)
    lock = resolve_inside(root, ".lock")
    with lock.open("a+b") as handle:
        os.chmod(lock, 0o600)
        try:
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise ImportFailure("storage.busy") from exc
        try:
            yield
        finally:
            fcntl.flock(handle, fcntl.LOCK_UN)


ALLOWED_DOCUMENTS = {"md", "txt", "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "csv", "html", "htm", "json", "xml"}


def extension(value: Any) -> str:
    result = str(value or "").lower().lstrip(".")
    if not re.fullmatch(r"[a-z0-9]{1,12}", result):
        raise ImportFailure("article.extension_invalid")
    return result


def validate_article(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict):
        raise ImportFailure("article.not_object")
    raw = dict(raw)
    if type(raw.get("articleCode")) is int:
        raw["articleCode"] = str(raw["articleCode"])
    for key in ("articleCode", "aritcleTitle"):
        if not isinstance(raw.get(key), str) or not raw[key].strip():
            raise ImportFailure("article.required_field_missing")
    if len(raw["articleCode"]) > 512:
        raise ImportFailure("article.id_too_long")
    value = raw.get("contentType")
    if type(value) not in (int, str) or str(value).strip() not in ("1", "2", "3"):
        raise ImportFailure("article.type_invalid")
    kind = int(value)
    for key in ("articleAbstract", "kmsUrl"):
        if raw.get(key) is not None and not isinstance(raw[key], str):
            raise ImportFailure("article.metadata_invalid")
    updated = raw.get("lastUpdateDate")
    if updated is not None and not isinstance(updated, str):
        if type(updated) not in (int, float) or not math.isfinite(updated):
            raise ImportFailure("article.metadata_invalid")
    if kind == 2:
        if not isinstance(raw.get("contentFileUuid"), str) or not raw["contentFileUuid"]:
            raise ImportFailure("article.main_file_missing")
        extension(raw.get("contentFileExtension"))
    elif not isinstance(raw.get("content"), str) or not raw["content"].strip():
        raise ImportFailure("article.content_missing")
    attachments = [] if raw.get("attachments") is None else raw["attachments"]
    if not isinstance(attachments, list):
        raise ImportFailure("article.attachments_invalid")
    for item in attachments:
        if not isinstance(item, dict) or not isinstance(item.get("s3uuid"), str) or not item["s3uuid"]:
            raise ImportFailure("article.attachment_invalid")
        extension(item.get("fileExtension"))
    return {**raw, "contentType": kind, "attachments": attachments}
