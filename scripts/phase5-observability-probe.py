#!/usr/bin/env python3
"""Collect redacted Phase 5 service, capacity, and security telemetry."""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPORT = ROOT / ".local/phase5-observability-report.json"
CONTAINERS = (
    "MindCreek-frontend", "MindCreek-gateway", "MindCreek-app", "MindCreek-docreader",
    "MindCreek-postgres", "MindCreek-redis", "MindCreek-mock-embedding",
)


def run(command: list[str]) -> str:
    result = subprocess.run(command, cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
    if result.returncode:
        raise RuntimeError(f"operator probe command failed: {' '.join(command[:3])}")
    return result.stdout.strip()


def metric_total(metrics: str, name: str, status: str = "") -> int:
    total = 0
    for line in metrics.splitlines():
        if not line.startswith(name):
            continue
        if status and f'status_class="{status}"' not in line:
            continue
        total += int(float(line.rsplit(" ", 1)[1]))
    return total


def env_value(name: str) -> str:
    for raw in (ROOT / ".local/mindcreek.env").read_text(encoding="utf-8").splitlines():
        if raw.strip() and not raw.lstrip().startswith("#") and "=" in raw:
            key, value = raw.split("=", 1)
            if key.strip() == name:
                return value.strip().strip('"').strip("'")
    return ""


def main() -> int:
    states: dict[str, str] = {}
    for container in CONTAINERS:
        raw = run(["docker", "inspect", container, "--format", "{{json .State}}"])
        state = json.loads(raw)
        status = state.get("Health", {}).get("Status", state.get("Status", "unknown"))
        states[container] = str(status)
        if status not in ("healthy", "running"):
            raise RuntimeError(f"service is not healthy: {container}")

    metrics = run(["docker", "exec", "MindCreek-frontend", "wget", "-qO-", "http://gateway:8080/internal/metrics"])
    if "mindcreek_gateway_in_flight_requests" not in metrics:
        raise RuntimeError("gateway metrics are unavailable")
    total = metric_total(metrics, "mindcreek_gateway_http_requests_total")
    failures = metric_total(metrics, "mindcreek_gateway_http_requests_total", "5xx")
    rate = failures / total if total else 0.0
    maximum_rate = float(os.environ.get("MINDCREEK_ALERT_MAX_5XX_RATE", "0.01"))
    if total >= 100 and rate > maximum_rate:
        raise RuntimeError(f"gateway 5xx rate exceeds threshold: {rate:.4f}")

    security_denials = sum(
        int(float(line.rsplit(" ", 1)[1]))
        for line in metrics.splitlines()
        if line.startswith("mindcreek_gateway_security_denials_total")
    )
    stats = []
    raw_stats = run(["docker", "stats", "--no-stream", "--format", "{{json .}}", *CONTAINERS])
    for line in raw_stats.splitlines():
        item = json.loads(line)
        stats.append({"name": item.get("Name"), "cpu": item.get("CPUPerc"), "memory": item.get("MemPerc")})

    redis_password = env_value("REDIS_PASSWORD")
    if not redis_password or run(["docker", "exec", "MindCreek-redis", "redis-cli", "--no-auth-warning", "-a", redis_password, "ping"]) != "PONG":
        raise RuntimeError("Redis readiness failed")
    database = run(["docker", "exec", "MindCreek-postgres", "pg_isready"])
    if "accepting connections" not in database:
        raise RuntimeError("PostgreSQL readiness failed")
    free_kb = int(run(["df", "-Pk", str(ROOT / ".local")]).splitlines()[-1].split()[3])
    minimum_kb = int(os.environ.get("MINDCREEK_ALERT_MIN_FREE_KB", "1048576"))
    if free_kb < minimum_kb:
        raise RuntimeError("backup/runtime filesystem free capacity is below threshold")

    report = {
        "status": "pass", "services": states, "gateway_requests": total,
        "gateway_5xx": failures, "gateway_5xx_rate": round(rate, 6),
        "security_denials": security_denials, "capacity": stats,
        "runtime_free_kb": free_kb, "postgres": "ready", "redis": "ready",
        "sensitive_payloads_collected": False,
    }
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    REPORT.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    os.chmod(REPORT, 0o600)
    print("Phase 5 observability passed: health, metrics, capacity, security counters, and redaction")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError, re.error) as error:
        print(f"Phase 5 observability probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
