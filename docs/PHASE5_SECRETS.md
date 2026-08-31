# Phase 5 Secret Lifecycle

MindCreek keeps production values outside Git in `.local/mindcreek.env` or an approved secret injector. The local file must be owned by the service operator, mode `0600`, excluded from workstation backup, and copied to the server only through the organization's protected channel. Run `python3 scripts/phase5-secret-check.py --env-file .local/mindcreek.env` before every production start.

## Creation and custody

Generate independent high-entropy values for PostgreSQL, Redis, JWT, the OIDC client, the internal broker client, and every model provider. `SYSTEM_AES_KEY` is exactly 32 bytes and must be escrowed because losing it makes stored provider overrides unreadable. Store TLS private keys and the environment file separately from ordinary deployment artifacts. Grant read access only to the service account and two designated operators.

## Backup and recovery

The normal data bundle intentionally excludes live secrets. Back up secrets with the organization's encrypted secret manager and test their recovery quarterly. Record the secret version identifiers—not values—in the incident ticket. Keep the current and previous versions through the rollback window.

## Rotation order

1. Take and verify a data backup.
2. Rotate model and corporate-OIDC credentials at the provider; install and probe the new version before revoking the old one.
3. Rotate the broker secret during a maintenance window; existing OIDC exchanges are invalidated.
4. Rotate JWT to revoke Web/MCP sessions. Rotate Redis and PostgreSQL credentials with coordinated service restarts.
5. Rotate `SYSTEM_AES_KEY` only with an export/re-encryption plan for workspace overrides.
6. Run Gate A, Gate B, the runtime check, and a browser login after rotation.

## Leak response

Disable the exposed credential at its issuer first, suspend affected sessions, preserve redacted audit evidence, rotate dependent secrets, and inspect Git/artifact history without copying the value into tickets or chat. Restore service from a known-good configuration and document the request/correlation IDs involved.
