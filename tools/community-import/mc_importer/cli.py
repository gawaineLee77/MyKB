"""Small resumable commands; no corporate endpoint is contacted by init or status."""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
from pathlib import Path

from .common import (ImportFailure, atomic_bytes, canonical, load_config, private_dir, resolve_inside,
                     safe_name, workspace_lock)
from .publish import Publisher, create_knowledge_base, destination_key, upload_counts
from .source import fetch_articles, local_articles
from .transport import S3Downloader, Transport
from .workspace import Workspace


def emit(stage: str, result: dict) -> None:
    print(json.dumps({"stage": stage, **result}, ensure_ascii=False), flush=True)


def fetch(workspace: Workspace, http: Transport) -> Path:
    destination = resolve_inside(workspace.root, "articles.jsonl")
    descriptor, temporary = tempfile.mkstemp(prefix=".fetch-", dir=workspace.root)
    count = 0
    try:
        with os.fdopen(descriptor, "wb") as handle:
            for article in fetch_articles(workspace.config, http):
                handle.write(canonical(article) + b"\n")
                count += 1
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, destination)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    emit("fetch", {"articles": count, "file": str(destination)})
    return destination


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description="Community API export and safe MindCreek document import (Python 3.12+, macOS/Linux).")
    result.add_argument("--config", type=Path, default=Path(".local/community-import.json"))
    result.add_argument("--output", type=Path, help="Protected work directory; default: output_dir in config")
    subs = result.add_subparsers(dest="command", required=True)
    subs.add_parser("init", help="Create a placeholder configuration; never overwrite an existing file")
    subs.add_parser("fetch", help="Fetch a complete API snapshot atomically, without uploading")
    subs.add_parser("status", help="Read local counts and sanitized error codes, without network access")
    sub = subs.add_parser("create-kb", help="Explicitly create/reconcile one native KB; prints its ID for configuration")
    sub.add_argument("--name", required=True)
    for name in ("prepare", "run"):
        sub = subs.add_parser(name, help="Prepare local JSON/JSONL" if name == "prepare" else "Fetch/prepare/upload (explicit remote write)")
        sub.add_argument("--input", type=Path, help="Already downloaded JSON array, single article, or JSONL; skips the article API")
        sub.add_argument("--refresh", action="store_true", help="Re-download resources even when source metadata is unchanged")
        if name == "run":
            sub.add_argument("--wait-seconds", type=float, default=0)
            sub.add_argument("--retry-failed", action="store_true")
    for name in ("upload", "check"):
        sub = subs.add_parser(name, help="Upload prepared files" if name == "upload" else "Check previously uploaded documents; no new uploads")
        sub.add_argument("--wait-seconds", type=float, default=0)
        if name == "upload":
            sub.add_argument("--retry-failed", action="store_true", help="Explicitly request reparsing failed/cancelled managed documents")
    sub = subs.add_parser("prune", help="Preview obsolete managed remote documents; local versions are always retained")
    sub.add_argument("--apply", action="store_true", help="DELETE the previewed obsolete owned documents after replacement readiness checks")
    return result


def execute(args: argparse.Namespace) -> int:
    if args.command == "init":
        if args.config.exists() or args.config.is_symlink():
            raise ImportFailure("config.already_exists")
        template = Path(__file__).resolve().parents[1] / "config.example.json"
        atomic_bytes(args.config, template.read_bytes())
        emit("init", {"config": str(args.config), "endpoints": "placeholders"})
        return 0
    config = load_config(args.config)
    output = args.output or Path(config.get("output_dir", ".local/community-import"))
    output = output.absolute()
    for parent in (output, *output.parents):
        if parent.is_symlink():
            if str(parent) not in ("/var", "/tmp") or parent.resolve() != Path("/private" + str(parent)):
                raise ImportFailure("storage.symlink_denied")
    output = output.resolve()
    if output in (Path("/"), Path.home().resolve(), Path.cwd().resolve()):
        raise ImportFailure("storage.broad_root_denied")
    root = output / safe_name(config["source_id"])
    wait = getattr(args, "wait_seconds", 0)
    if not 0 <= wait <= 86400:
        raise ImportFailure("config.wait_limit_invalid")
    with workspace_lock(root):
        workspace = Workspace(root, config)
        if args.command == "status":
            desired = workspace.desired()
            records = workspace.state["uploads"].get(destination_key(config), {})
            counts = upload_counts(workspace, desired, records)
            errors = [{"article": key, **value["error"]} for key, value in workspace.state["articles"].items() if value.get("error")]
            upload_errors = [{"artifact": key, **value["error"]} for key, value in records.items()
                             if key in desired and value.get("error")]
            emit("status", {"workspace": str(root), "articles": len(workspace.state["articles"]),
                            "prepared": sum(bool(item.get("manifest")) for item in workspace.state["articles"].values()),
                            "errors": errors, "upload_errors": upload_errors,
                            "last_prepare": workspace.state.get("last_prepare", {}),
                            "prepare_incomplete": bool(workspace.state.get("prepare_incomplete")),
                            "uploads": counts,
                            "note": "local snapshot only; run check for current remote parsing and authorization"})
            if counts["failed"] or counts["article_failures"]:
                return 1
            return 2 if counts["pending"] or counts["not_uploaded"] else 0
        http = Transport(config)
        if args.command == "create-kb":
            emit("create-kb", create_knowledge_base(workspace, http, args.name))
            return 0
        if args.command == "fetch":
            fetch(workspace, http)
            return 0
        prepare_failed = False
        if args.command in ("prepare", "run"):
            path = args.input
            if path is None:
                path = fetch(workspace, http) if args.command == "run" else root / "articles.jsonl"
            counts = workspace.prepare(local_articles(path), S3Downloader(http, config), refresh=args.refresh)
            emit("prepare", counts)
            prepare_failed = bool(counts["failed"])
            if args.command == "prepare":
                return 1 if prepare_failed else 0
        publisher = Publisher(workspace, http)
        if args.command == "prune":
            emit("prune", publisher.prune(apply=args.apply))
            return 0
        counts = publisher.upload(check_only=args.command == "check", wait_seconds=wait,
                                  retry_failed=getattr(args, "retry_failed", False))
        emit(args.command, counts)
        if prepare_failed or counts["failed"] or counts["article_failures"]:
            return 1
        return 2 if counts["pending"] or counts["not_uploaded"] else 0


def main(argv: list[str] | None = None) -> int:
    os.umask(0o077)
    args = parser().parse_args(argv)
    try:
        return execute(args)
    except ImportFailure as exc:
        emit("error", {"code": exc.code, "status": exc.status})
    except (OSError, ValueError, TypeError, KeyError, UnicodeError) as exc:
        # Never echo exception text: network URLs, API payloads, paths, and secrets
        # can be embedded in standard-library exception messages.
        emit("error", {"code": "operation.invalid_input_or_local_io", "exception_type": type(exc).__name__})
    except KeyboardInterrupt:
        emit("error", {"code": "operation.interrupted", "note": "resume the same workspace"})
        return 130
    return 1
