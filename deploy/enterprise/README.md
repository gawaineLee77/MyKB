# Current AMD64 enterprise bundle

The runtime supports fresh installations and controlled upgrades of the pinned
WeKnora v0.8.0 workspace-centric enterprise deployment. It includes R5 employee
assistants, the native graph browser and editable Word export. Use the
[Chinese install/upgrade guide](../../docs/guides/ENTERPRISE_DEPLOYMENT_AND_UPGRADE_ZH.md).

The server needs Docker/Compose and Python 3.10+. All nine AMD64 images are
included, including optional Neo4j/APOC; model weights and company secrets are
not included. Existing Phase 5/v0.7.2 databases are not an in-place upgrade source.

## Release tooling

- `tools/release/build_current.py`: current verified Node 24 frontend and Go 1.26
  sources, isolated image contexts, nine locked AMD64 images.
- `tools/release/package_current.py assemble|export|finalize`: immutable package
  with checksums, docs, CA roots, install/upgrade/recovery tools and evidence.
- `tools/release/verify_archive.py`: reload the exported tar and check every image.
- `tools/release/smoke_enterprise.py` with `MINDCREEK_CURRENT_SMOKE=true`: actual
  images, isolated synthetic OAuth/model services and backup/recovery.
- Current generated artifacts are under `.local/enterprise-current/` and ignored
  `images/archives/`. Set `MINDCREEK_RELEASE_WORKDIR` to the current directory for
  smoke/archive tools. Original R4/R5 reports and release archives stay intact.

`bin/release plan` is read-only. `upgrade` preflights configuration, stops the
identified project, creates a cold backup, replaces only recognized historical
image pins/policies and starts the new runtime. Backups include actual product
images used by old add-ons. `restore` accepts only a new project/state and the
matching release, and does not start services. No operation deletes live volumes.

Local synthetic acceptance is separate from R6 company deployment. In particular,
physical AMD64 BM25/database checks, real OAuth/model services, proxy behavior and
Word edit/save need target-environment acceptance; read the package's limitations.
