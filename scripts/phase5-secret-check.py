#!/usr/bin/env python3
"""Validate Phase 5 secret inputs without printing their values."""

from __future__ import annotations

import argparse
import os
import stat
from pathlib import Path
from urllib.parse import urlparse


SECRET_FIELDS = (
    "DB_PASSWORD",
    "REDIS_PASSWORD",
    "JWT_SECRET",
    "SYSTEM_AES_KEY",
    "MINDCREEK_MANAGED_LLM_API_KEY",
    "MINDCREEK_MANAGED_EMBEDDING_API_KEY",
    "MINDCREEK_MANAGED_RERANK_API_KEY",
)
PLACEHOLDERS = ("replace", "development-only", "changeme", "example", "password")


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if line and not line.startswith("#") and "=" in line:
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def require_secret(values: dict[str, str], name: str, minimum: int = 24) -> str:
    value = values.get(name, "")
    if len(value) < minimum or any(marker in value.lower() for marker in PLACEHOLDERS):
        raise ValueError(f"{name} is missing, too short, or still a placeholder")
    return value


def validate_file_mode(path: Path, maximum: int) -> None:
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"protected file must be a regular non-symlink: {path}")
    mode = stat.S_IMODE(path.stat().st_mode)
    if mode & ~maximum:
        raise ValueError(f"protected file permissions are too broad: {path} ({mode:o})")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--env-file", type=Path, required=True)
    parser.add_argument("--allow-development", action="store_true")
    args = parser.parse_args()
    validate_file_mode(args.env_file, 0o600)
    values = load_env(args.env_file)
    production = values.get("MINDCREEK_DEPLOYMENT_ENV", "development").lower() == "production"
    if not production and not args.allow_development:
        raise ValueError("MINDCREEK_DEPLOYMENT_ENV must be production")

    secrets = [require_secret(values, name) for name in SECRET_FIELDS]
    if len(values.get("SYSTEM_AES_KEY", "").encode("utf-8")) != 32:
        raise ValueError("SYSTEM_AES_KEY must contain exactly 32 UTF-8 bytes")
    if len(set(secrets)) != len(secrets):
        raise ValueError("database, Redis, JWT, AES, and model credentials must be distinct")

    for name in (
        "MINDCREEK_MANAGED_LLM_BASE_URL",
        "MINDCREEK_MANAGED_EMBEDDING_BASE_URL",
        "MINDCREEK_MANAGED_RERANK_BASE_URL",
    ):
        parsed = urlparse(values.get(name, ""))
        if parsed.scheme != "https" or not parsed.hostname:
            raise ValueError(f"{name} must be an absolute HTTPS URL in production")

    if values.get("MINDCREEK_IDENTITY_ENABLED", "false").lower() != "true":
        raise ValueError("corporate identity must be enabled in the production pilot")
    require_secret(values, "MINDCREEK_IDENTITY_CLIENT_SECRET", 16)
    require_secret(values, "MINDCREEK_BROKER_CLIENT_SECRET", 32)
    origin = urlparse(values.get("MINDCREEK_EXTERNAL_ORIGIN", ""))
    if origin.scheme != "https" or not origin.hostname or origin.path not in ("", "/"):
        raise ValueError("MINDCREEK_EXTERNAL_ORIGIN must be an HTTPS origin")

    cert = Path(values.get("MINDCREEK_TLS_CERT_FILE", ""))
    key = Path(values.get("MINDCREEK_TLS_KEY_FILE", ""))
    if not cert.is_absolute() or not key.is_absolute():
        raise ValueError("TLS certificate and key paths must be absolute host paths")
    if not cert.is_file():
        raise ValueError("MINDCREEK_TLS_CERT_FILE does not exist")
    validate_file_mode(key, 0o600)
    print(f"Phase 5 production secrets verified: {len(SECRET_FIELDS) + 2} credential classes; values redacted")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError) as error:
        print(f"Phase 5 secret check failed: {error}", file=os.sys.stderr)
        raise SystemExit(1)
