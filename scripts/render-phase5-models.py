#!/usr/bin/env python3
"""Validate Phase 5 model settings and render a secret-free built-in YAML."""

from __future__ import annotations

import argparse
import os
import re
import stat
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
TEMPLATE = ROOT / "deploy/phase5/builtin_models.yaml.tmpl"

DEV_DEFAULTS = {
    "MINDCREEK_MANAGED_LLM_NAME": "mindcreek-test-chat",
    "MINDCREEK_MANAGED_LLM_BASE_URL": "http://mock-embedding:19090/v1",
    "MINDCREEK_MANAGED_LLM_API_KEY": "development-only",
    "MINDCREEK_MANAGED_LLM_PROVIDER": "generic",
    "MINDCREEK_MANAGED_EMBEDDING_NAME": "mindcreek-test-embedding",
    "MINDCREEK_MANAGED_EMBEDDING_BASE_URL": "http://mock-embedding:19090/v1",
    "MINDCREEK_MANAGED_EMBEDDING_API_KEY": "development-only",
    "MINDCREEK_MANAGED_EMBEDDING_PROVIDER": "generic",
    "MINDCREEK_MANAGED_EMBEDDING_DIMENSION": "64",
    "MINDCREEK_MANAGED_RERANK_NAME": "mindcreek-test-rerank",
    "MINDCREEK_MANAGED_RERANK_BASE_URL": "http://mock-embedding:19090/v1",
    "MINDCREEK_MANAGED_RERANK_API_KEY": "development-only",
    "MINDCREEK_MANAGED_RERANK_PROVIDER": "generic",
}


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.exists():
        return values
    for number, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError(f"invalid environment entry on line {number}")
        key, value = line.split("=", 1)
        key = key.strip()
        if key:
            values[key] = value.strip().strip('"').strip("'")
    return values


def setting(values: dict[str, str], name: str, development: bool) -> str:
    value = values.get(name, os.environ.get(name, "")).strip()
    if not value and development:
        value = DEV_DEFAULTS.get(name, "")
    if not value:
        raise ValueError(f"{name} is required")
    return value


def optional_setting(values: dict[str, str], name: str, fallback: str = "") -> str:
    return values.get(name, os.environ.get(name, fallback)).strip()


def validate_url(name: str, value: str, production_like: bool, allow_http: bool) -> None:
    parsed = urlsplit(value)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError(f"{name} must be an absolute HTTP(S) URL without userinfo, query, or fragment")
    if production_like and parsed.scheme != "https" and not allow_http:
        raise ValueError(f"{name} must use HTTPS unless MINDCREEK_MANAGED_ALLOW_HTTP=true")
    if production_like and any(marker in parsed.hostname.lower() for marker in ("mock-embedding", "example.invalid", "localhost")):
        raise ValueError(f"{name} cannot use a test endpoint outside development")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--env-file", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    values = load_env(args.env_file)
    deployment = values.get("MINDCREEK_DEPLOYMENT_ENV", os.environ.get("MINDCREEK_DEPLOYMENT_ENV", "development")).strip().lower()
    if deployment not in {"development", "staging", "production"}:
        raise ValueError("MINDCREEK_DEPLOYMENT_ENV must be development, staging, or production")
    development = deployment == "development"
    production_like = not development
    allow_http = optional_setting(values, "MINDCREEK_MANAGED_ALLOW_HTTP", "false").lower() == "true"

    required = {name: setting(values, name, development) for name in DEV_DEFAULTS}
    dimension = int(required["MINDCREEK_MANAGED_EMBEDDING_DIMENSION"])
    if dimension < 1 or dimension > 65536:
        raise ValueError("MINDCREEK_MANAGED_EMBEDDING_DIMENSION must be between 1 and 65536")
    for prefix in ("LLM", "EMBEDDING", "RERANK"):
        model_name = required[f"MINDCREEK_MANAGED_{prefix}_NAME"]
        provider = required[f"MINDCREEK_MANAGED_{prefix}_PROVIDER"]
        if not model_name.strip() or any(character in model_name for character in "\r\n"):
            raise ValueError(f"MINDCREEK_MANAGED_{prefix}_NAME is invalid")
        if not re.fullmatch(r"[a-z0-9_-]{1,64}", provider):
            raise ValueError(f"MINDCREEK_MANAGED_{prefix}_PROVIDER must be a lowercase provider identifier")
        validate_url(
            f"MINDCREEK_MANAGED_{prefix}_BASE_URL",
            required[f"MINDCREEK_MANAGED_{prefix}_BASE_URL"],
            production_like,
            allow_http,
        )
        if production_like and any(marker in required[f"MINDCREEK_MANAGED_{prefix}_API_KEY"].lower() for marker in ("development", "replace", "example")):
            raise ValueError(f"MINDCREEK_MANAGED_{prefix}_API_KEY is a placeholder")

    aes_key = optional_setting(values, "SYSTEM_AES_KEY")
    if production_like:
        if len(aes_key.encode("utf-8")) != 32:
            raise ValueError("managed models require a 32-byte SYSTEM_AES_KEY outside development")
        if any(marker in aes_key.lower() for marker in ("0123456789", "phase0", "replace", "example")):
            raise ValueError("managed models require a non-placeholder SYSTEM_AES_KEY outside development")

    if optional_setting(values, "MINDCREEK_USER_MODEL_OVERRIDES", "false").lower() == "true":
        if len(aes_key.encode("utf-8")) != 32:
            raise ValueError("user model overrides require a 32-byte SYSTEM_AES_KEY")
        if not optional_setting(values, "MINDCREEK_MODEL_OVERRIDE_HOSTS"):
            raise ValueError("user model overrides require MINDCREEK_MODEL_OVERRIDE_HOSTS")

    rendered = TEMPLATE.read_text(encoding="utf-8").replace("__MINDCREEK_EMBEDDING_DIMENSION__", str(dimension))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(rendered, encoding="utf-8")
    os.chmod(args.output, stat.S_IRUSR | stat.S_IWUSR)
    print(f"Rendered 3 managed model declarations for {deployment}; credentials were not written or printed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
