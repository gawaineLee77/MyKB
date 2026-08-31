# Phase 5 Observability and Alerts

The gateway emits one JSON event per request with only method, bounded route class, status, duration, and correlation ID. It never records URLs, query strings, authorization headers, prompts, answers, excerpts, model credentials, or document content. The private `GET /internal/metrics` endpoint exposes request totals, duration sums, in-flight capacity, and 401/403/429 security-denial counters; it is reachable only on the internal container network.

Run the redacted operator probe after deployment and after incidents:

```sh
python3 scripts/phase5-observability-probe.py
```

The default alert policy is:

- any unhealthy service: immediate page;
- HTTP 5xx rate above 1% after at least 100 requests: page;
- less than 1 GiB free in the runtime/backup filesystem: page;
- repeated 401/403/429 growth: investigate identity, abuse, or client configuration;
- managed model unavailable or Gate A probe failure: disable new ingestion and notify the model operator;
- backup older than the configured 24-hour RPO or recovery drill above 30-minute RTO: page.

Container CPU/memory snapshots are capacity signals, not long-term storage. Forward stdout and metrics to the organization's approved system with retention and access controls. Search incidents by `X-Request-ID`; do not enable `LLM_DEBUG_LOG` in shared environments.
