#!/usr/bin/env python3
"""Run a bounded concurrent load smoke test against the public gateway path."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import statistics
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def percentile(values: list[float], fraction: float) -> float:
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, int((len(ordered) - 1) * fraction))]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.environ.get("MINDCREEK_BASE_URL", "http://127.0.0.1:18080"))
    parser.add_argument("--requests", type=int, default=300)
    parser.add_argument("--concurrency", type=int, default=20)
    parser.add_argument("--p95-ms", type=float, default=750)
    parser.add_argument("--report", default=str(ROOT / ".local/phase5-load-report.json"))
    args = parser.parse_args()
    if not 1 <= args.concurrency <= 64 or not 10 <= args.requests <= 5000:
        raise ValueError("load bounds are outside the controlled smoke-test range")
    target = args.base_url.rstrip("/") + "/api/v1/capabilities/knowledge-modes"

    def request(index: int) -> tuple[bool, float]:
        started = time.perf_counter()
        value = urllib.request.Request(target, headers={"X-Request-ID": f"phase5-load-{index}"})
        try:
            with urllib.request.urlopen(value, timeout=10) as response:
                body = response.read(128 * 1024 + 1)
                ok = response.status == 200 and len(body) <= 128 * 1024
        except OSError:
            ok = False
        return ok, (time.perf_counter() - started) * 1000

    started = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        results = list(executor.map(request, range(args.requests)))
    duration = time.perf_counter() - started
    latencies = [latency for _, latency in results]
    failures = sum(not ok for ok, _ in results)
    p95 = percentile(latencies, 0.95)
    error_rate = failures / args.requests
    if error_rate > 0.01 or p95 > args.p95_ms:
        raise RuntimeError(f"load target missed: errors={error_rate:.3f}, p95={p95:.1f}ms")
    report = {
        "status": "pass", "requests": args.requests, "concurrency": args.concurrency,
        "error_rate": error_rate, "p50_ms": round(statistics.median(latencies), 2),
        "p95_ms": round(p95, 2), "throughput_rps": round(args.requests / duration, 2),
        "response_body_retained": False,
    }
    path = Path(args.report)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    os.chmod(path, 0o600)
    print(f"Phase 5 load probe passed: {args.requests} requests, p95={p95:.1f}ms, errors={failures}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, RuntimeError) as error:
        print(f"Phase 5 load probe failed: {error}", file=sys.stderr)
        raise SystemExit(1)
