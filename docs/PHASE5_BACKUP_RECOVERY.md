# Phase 5 Backup and Recovery

The default objective for the controlled pilot is RPO 24 hours and RTO 30 minutes. Adjust `MINDCREEK_BACKUP_RPO_HOURS` and `MINDCREEK_BACKUP_RTO_MINUTES` only with an approved service target.

## Consistent backup

`scripts/phase5-backup.sh` briefly quiesces the UI, gateway, application, and document reader while PostgreSQL creates a custom-format dump and the local object volume is archived. It also stores product configuration, migration inventory, source revisions, and checksums. Live secrets are deliberately excluded and must be recovered from the approved secret manager.

```sh
./scripts/phase5-backup.sh
./scripts/phase5-recovery-drill.sh
```

Copy the resulting mode-`0700` directory to encrypted, access-controlled storage. Schedule the backup at least every 24 hours and retain daily, weekly, and pre-upgrade copies according to organizational policy.

## Restore

Validate the recovery drill first. On the replacement host, load the matching Phase 5 image bundle, install the protected secrets, and create an empty runtime. The restore command replaces the current database and uploaded-file volume and therefore requires an explicit flag:

```sh
./scripts/phase5-restore.sh --confirm-replace-current-data /protected/backup-directory
make phase5-runtime-check
make phase5-gate-a
```

Rebuild derived indexes only if the restored database reports incomplete indexing; do not silently change an existing KB's embedding model. Verify counts, one Personal Note, one Plain RAG document, one citation, one subscription, and one MCP query before reopening access. Preserve the previous data and image versions until the observation window closes.
