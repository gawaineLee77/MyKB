#!/usr/bin/env python3
"""Verify MindCreek migrations without touching the live application database."""

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
COMPOSE = ROOT / "scripts/phase1-compose.sh"
POSTGRES_CONTAINER = "MindCreek-postgres"


def fail(message: str) -> None:
    raise RuntimeError(message)


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def run(command: list[str], *, input_text: str | None = None) -> str:
    result = subprocess.run(
        command,
        cwd=ROOT,
        input=input_text,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if result.returncode != 0:
        tail = "\n".join(result.stdout.splitlines()[-20:])
        fail(f"command failed ({result.returncode}): {' '.join(command[:5])}\n{tail}")
    return result.stdout.strip()


def psql_scalar(user: str, database: str, sql: str) -> str:
    return run(
        [
            "docker",
            "exec",
            POSTGRES_CONTAINER,
            "psql",
            "-U",
            user,
            "-d",
            database,
            "-Atqc",
            sql,
        ]
    )


def migrate(database_url: str, *arguments: str) -> str:
    return run(
        [
            str(COMPOSE),
            "run",
            "--rm",
            "--no-deps",
            "-e",
            f"MINDCREEK_DATABASE_URL={database_url}",
            "gateway",
            "migrate",
            *arguments,
        ]
    )


def main() -> int:
    if not ENV_FILE.exists():
        fail(f"runtime environment is missing: {ENV_FILE}")
    values = load_env(ENV_FILE)
    user = values.get("DB_USER", "")
    password = values.get("DB_PASSWORD", "")
    primary_database = values.get("DB_NAME", "")
    for label, value in (("DB_USER", user), ("DB_NAME", primary_database)):
        if not re.fullmatch(r"[A-Za-z0-9_]+", value):
            fail(f"{label} contains unsupported characters")

    temporary_database = f"mindcreek_gate_b_{secrets.token_hex(5)}"
    database_url = (
        "postgres://"
        + urllib.parse.quote(user, safe="")
        + ":"
        + urllib.parse.quote(password, safe="")
        + "@postgres:5432/"
        + temporary_database
        + "?sslmode=disable"
    )
    public_tables_before = psql_scalar(
        user,
        primary_database,
        "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'",
    )
    run(
        [
            "docker",
            "exec",
            POSTGRES_CONTAINER,
            "createdb",
            "-U",
            user,
            temporary_database,
        ]
    )
    temporary_public_tables_before = psql_scalar(
        user,
        temporary_database,
        "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'",
    )

    try:
        migrate(database_url, "up")
        first_count = psql_scalar(
            user, temporary_database, "SELECT count(*) FROM mindcreek.schema_migrations"
        )
        if first_count != "8":
            fail(f"empty install applied {first_count} migrations, expected 8")

        migrate(database_url, "up")
        repeat_count = psql_scalar(
            user, temporary_database, "SELECT count(*) FROM mindcreek.schema_migrations"
        )
        if repeat_count != "8":
            fail("repeat migration was not idempotent")

        status = migrate(database_url, "status")
        if any(
            expected not in status
            for expected in (
                "000001 runtime_metadata",
                "000002 kb_profiles",
                "000003 knowledge_space_requests",
                "000004 note_revisions",
                "000005 index_profiles",
                "000006 kb_access_grants",
                "000007 phase2_security_records",
                "000008 phase3_publications",
            )
        ):
            fail("migration status did not report every product migration")

        migrate(database_url, "down", "8")
        rolled_back = psql_scalar(
            user,
            temporary_database,
            "SELECT concat(to_regclass('mindcreek.runtime_metadata'), '|', "
            "to_regclass('mindcreek.kb_profiles'), '|', "
            "to_regclass('mindcreek.knowledge_space_requests'), '|', "
            "to_regclass('mindcreek.note_revisions'), '|', "
            "to_regclass('mindcreek.index_profiles'), '|', "
            "to_regclass('mindcreek.kb_access_grants'), '|', "
            "to_regclass('mindcreek.session_kb_scopes'), '|', "
            "to_regclass('mindcreek.kb_access_audit_events'), '|', "
            "to_regclass('mindcreek.kb_publications'), '|', "
            "to_regclass('mindcreek.kb_subscriptions'), '|', "
            "to_regclass('mindcreek.kb_content_revisions'), '|', "
            "to_regclass('mindcreek.kb_activity_events'), '|', "
            "(SELECT count(*) FROM mindcreek.schema_migrations))",
        )
        if rolled_back != "||||||||||||0":
            fail(f"rollback left unexpected product state: {rolled_back!r}")

        migrate(database_url, "up")
        forward_count = psql_scalar(
            user, temporary_database, "SELECT count(*) FROM mindcreek.schema_migrations"
        )
        if forward_count != "8":
            fail("forward migration after rollback did not restore every version")

        temporary_public_tables_after = psql_scalar(
            user,
            temporary_database,
            "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'",
        )
        if temporary_public_tables_after != temporary_public_tables_before:
            fail("product migrations changed the temporary database public schema")

        public_tables_after = psql_scalar(
            user,
            primary_database,
            "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'",
        )
        if public_tables_after != public_tables_before:
            fail("the upstream public schema changed during the isolated migration probe")

        report = {
            "empty_install": "pass",
            "repeat_startup": "pass",
            "rollback_forward": "pass",
            "upstream_public_table_count_unchanged": True,
            "temporary_public_schema_unchanged": True,
            "live_public_schema_unchanged": True,
            "applied_migrations": 8,
        }
        report_path = ROOT / ".local/phase1-migration-probe-report.json"
        report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        print("MindCreek migration lifecycle passed: empty, repeat, rollback, forward")
        return 0
    finally:
        run(
            [
                "docker",
                "exec",
                POSTGRES_CONTAINER,
                "dropdb",
                "-U",
                user,
                "--if-exists",
                temporary_database,
            ]
        )


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, RuntimeError) as error:
        print(f"MindCreek migration probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
