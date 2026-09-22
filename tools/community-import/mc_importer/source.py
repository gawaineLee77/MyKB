"""Configurable list/detail adapters and local JSON/JSONL inputs."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Iterator

from .common import ImportFailure, canonical, digest, read_json, select
from .transport import Transport


def local_articles(path: Path) -> Iterator[dict[str, Any]]:
    if path.suffix.lower() == ".jsonl":
        with path.open(encoding="utf-8") as handle:
            for line in handle:
                if line.strip():
                    try:
                        yield json.loads(line)
                    except ValueError as exc:
                        raise ImportFailure("input.invalid_jsonl") from exc
    else:
        value = read_json(path)
        if isinstance(value, dict) and "articleCode" in value:
            value = [value]
        if not isinstance(value, list):
            raise ImportFailure("input.expected_article_array")
        yield from value


def fetch_articles(config: dict[str, Any], http: Transport) -> Iterator[dict[str, Any]]:
    source = config.get("articles", {})
    paging = source.get("pagination", {})
    mode = paging.get("mode", "none")
    if mode not in ("none", "page", "offset", "cursor"):
        raise ImportFailure("config.pagination_mode_invalid")
    page_size = int(paging.get("page_size", 100))
    max_pages = int(paging.get("max_pages", 10000))
    if not 1 <= page_size <= 10000 or not 1 <= max_pages <= 100000:
        raise ImportFailure("config.pagination_limits_invalid")
    variables = {"page": int(paging.get("start_page", 1)), "page_size": page_size,
                 "offset": int(paging.get("start_offset", 0)), "cursor": paging.get("start_cursor", "")}
    fingerprints, cursors = set(), set()
    for _ in range(max_pages):
        response = http.json_request(source.get("request", {}), variables)
        items = select(response, source.get("items_path", "data.items"))
        if isinstance(items, dict) and mode == "none":
            items = [items]
        if not isinstance(items, list):
            raise ImportFailure("source.items_not_array")
        fingerprint = digest(canonical(items))
        if items and fingerprint in fingerprints:
            raise ImportFailure("source.repeated_page")
        if items:
            fingerprints.add(fingerprint)
        for item in items:
            if not isinstance(item, dict):
                raise ImportFailure("source.article_not_object")
            detail = source.get("detail")
            if detail:
                if type(item.get("articleCode")) not in (str, int):
                    raise ImportFailure("source.detail_id_missing")
                value = http.json_request(detail["request"], {"articleCode": item["articleCode"]})
                detail_item = select(value, detail.get("item_path", "data"))
                if not isinstance(detail_item, dict) or str(detail_item.get("articleCode")) != str(item["articleCode"]):
                    raise ImportFailure("source.detail_identity_mismatch")
                item = detail_item
            yield item
        if mode == "none":
            return
        if mode == "cursor":
            cursor = select(response, paging.get("next_cursor_path", "data.nextCursor"))
            if cursor in (None, ""):
                return
            key = str(cursor)
            if key in cursors or key == str(variables["cursor"]):
                raise ImportFailure("source.repeated_cursor")
            cursors.add(key)
            variables["cursor"] = cursor
        else:
            has_more = paging.get("has_more_path")
            if has_more:
                more = select(response, has_more)
                if not isinstance(more, bool):
                    raise ImportFailure("source.has_more_not_boolean")
                if not more:
                    return
            elif len(items) < page_size:
                return
            variables["page"] += 1
            variables["offset"] += page_size
    raise ImportFailure("source.page_limit_reached")
