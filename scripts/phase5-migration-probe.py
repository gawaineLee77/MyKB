#!/usr/bin/env python3
"""Verify all ten product migrations on an isolated temporary database."""

from __future__ import annotations

import json
import re
import secrets
import subprocess
import sys
import urllib.parse
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ENV_FILE = ROOT / ".local/mindcreek.env"
COMPOSE = ROOT / "scripts/phase5-compose.sh"


def run(command: list[str]) -> str:
    result = subprocess.run(command, cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
    if result.returncode:
        raise RuntimeError(f"migration command failed: {' '.join(command[:4])}\n" + "\n".join(result.stdout.splitlines()[-10:]))
    return result.stdout.strip()


def env_values() -> dict[str, str]:
    values = {}
    for raw in ENV_FILE.read_text(encoding="utf-8").splitlines():
        if raw.strip() and not raw.lstrip().startswith("#") and "=" in raw:
            key, value = raw.split("=", 1)
            values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def scalar(user: str, database: str, sql: str) -> str:
    return run(["docker", "exec", "MindCreek-postgres", "psql", "-U", user, "-d", database, "-Atqc", sql])


def main() -> int:
    values = env_values()
    user, password = values.get("DB_USER", ""), values.get("DB_PASSWORD", "")
    primary = values.get("DB_NAME", "")
    if not re.fullmatch(r"[A-Za-z0-9_]+", user) or not re.fullmatch(r"[A-Za-z0-9_]+", primary):
        raise RuntimeError("database identifiers contain unsupported characters")
    temporary = "mindcreek_phase5_" + secrets.token_hex(5)
    url = "postgres://" + urllib.parse.quote(user, safe="") + ":" + urllib.parse.quote(password, safe="") + "@postgres:5432/" + temporary + "?sslmode=disable"
    public_before = scalar(user, primary, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")
    run(["docker", "exec", "MindCreek-postgres", "createdb", "-U", user, temporary])
    try:
        def migrate(*args: str) -> str:
            return run([str(COMPOSE), "run", "--rm", "--no-deps", "-e", f"MINDCREEK_DATABASE_URL={url}", "gateway", "migrate", *args])
        migrate("up")
        if scalar(user, temporary, "SELECT count(*) FROM mindcreek.schema_migrations") != "10":
            raise RuntimeError("empty install did not apply ten migrations")
        if scalar(user, temporary, "SELECT concat(to_regclass('mindcreek.corporate_identities'),'|',to_regclass('mindcreek.identity_audit_events'))") != "mindcreek.corporate_identities|mindcreek.identity_audit_events":
            raise RuntimeError("corporate identity schema is missing")
        migrate("up")
        migrate("down", "10")
        if scalar(user, temporary, "SELECT count(*) FROM mindcreek.schema_migrations") != "0":
            raise RuntimeError("full rollback left product migrations")
        migrate("up")
        if scalar(user, temporary, "SELECT count(*) FROM mindcreek.schema_migrations") != "10":
            raise RuntimeError("forward migration after rollback failed")
        if scalar(user, primary, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'") != public_before:
            raise RuntimeError("isolated migration changed the live upstream schema")
        report = {"status": "pass", "empty": True, "repeat": True, "rollback_forward": True, "migrations": 10, "live_public_schema_unchanged": True}
        path = ROOT / ".local/phase5-migration-report.json"
        path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        path.chmod(0o600)
        print("Phase 5 migration lifecycle passed: ten migrations, repeat, rollback, and forward")
        return 0
    finally:
        run(["docker", "exec", "MindCreek-postgres", "dropdb", "-U", user, "--if-exists", temporary])


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, RuntimeError) as error:
        print(f"Phase 5 migration probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
