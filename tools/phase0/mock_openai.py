#!/usr/bin/env python3
"""Deterministic OpenAI-compatible embedding endpoint for Phase 0 tests."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import re
import unicodedata
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

DIMENSION = 64
MAX_BODY_BYTES = 2 * 1024 * 1024


def _features(text: str) -> list[str]:
    normalized = unicodedata.normalize("NFKC", text).lower()
    compact = re.sub(r"\s+", " ", normalized).strip()
    words = re.findall(r"[a-z0-9_]+|[\u3400-\u9fff]", compact)
    dense = re.sub(r"\s+", "", compact)
    ngrams = [dense[index : index + 3] for index in range(max(0, len(dense) - 2))]
    return words + ngrams


def embed(text: str) -> list[float]:
    vector = [0.0] * DIMENSION
    for feature in _features(text):
        digest = hashlib.sha256(feature.encode("utf-8")).digest()
        index = int.from_bytes(digest[:4], "big") % DIMENSION
        sign = 1.0 if digest[4] & 1 else -1.0
        vector[index] += sign
    magnitude = math.sqrt(sum(value * value for value in vector))
    if magnitude == 0:
        vector[0] = 1.0
        return vector
    return [value / magnitude for value in vector]


class Handler(BaseHTTPRequestHandler):
    server_version = "MyKBPhase0Embedding/1.0"

    def _json(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/health":
            self._json(200, {"status": "ok", "dimension": DIMENSION})
            return
        if self.path.rstrip("/").endswith("/models"):
            self._json(
                200,
                {
                    "object": "list",
                    "data": [{"id": "mykb-phase0-embedding", "object": "model"}],
                },
            )
            return
        self._json(404, {"error": {"message": "not found"}})

    def do_POST(self) -> None:  # noqa: N802
        if not self.path.rstrip("/").endswith("/embeddings"):
            self._json(404, {"error": {"message": "not found"}})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > MAX_BODY_BYTES:
                raise ValueError("invalid request size")
            payload = json.loads(self.rfile.read(length))
            raw_input = payload.get("input", [])
            inputs = [raw_input] if isinstance(raw_input, str) else list(raw_input)
            data = [
                {"object": "embedding", "index": index, "embedding": embed(str(value))}
                for index, value in enumerate(inputs)
            ]
            token_count = sum(len(_features(str(value))) for value in inputs)
            self._json(
                200,
                {
                    "object": "list",
                    "data": data,
                    "model": payload.get("model", "mykb-phase0-embedding"),
                    "usage": {"prompt_tokens": token_count, "total_tokens": token_count},
                },
            )
        except (ValueError, TypeError, json.JSONDecodeError) as exc:
            self._json(400, {"error": {"message": str(exc)}})

    def log_message(self, format_string: str, *args: object) -> None:
        print(f"mock-embedding: {format_string % args}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=19090)
    args = parser.parse_args()
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"mock-embedding: listening on {args.host}:{args.port}")
    server.serve_forever()


if __name__ == "__main__":
    main()
