# Phase 5 Controlled Pilot

Phase 5 is intended for a small internal cohort, not an unrestricted production launch. The automated baseline uses only synthetic bilingual material and measures zero-key setup, ingestion, Recall@5, mean reciprocal rank, retrieval latency, grounded Ask, and openable citations:

```sh
python3 scripts/phase5-pilot-probe.py
```

## Team exercise

Select one knowledge owner and two readers from each pilot team. Use approved, non-sensitive representative documents. Each participant must complete corporate login, create or receive a KB, ingest content, find an English and Chinese answer, open its citation, subscribe/unsubscribe, and—where approved—query the same scope through MCP.

Record only aggregate scores: task completion, retrieval relevance (0–3), groundedness (0–3), p50/p95 latency, provider token/cost totals, and blocked/incorrect-answer counts. Never paste prompts, answers, excerpts, employee identifiers, or API keys into the scorecard. A team passes when task completion is at least 90%, Recall@5 at least 0.90, groundedness at least 0.90, and no authorization escape occurs.

## Release decision

Engineering acceptance authorizes this controlled exercise. Broad rollout still requires the corporate IdP browser test, selected-team sign-off, production provider cost review, backup restore evidence on the target server, and an incident owner. Any scope leak, ungrounded sensitive answer, unavailable default model, missed RPO/RTO, or fixable critical vulnerability blocks expansion.
