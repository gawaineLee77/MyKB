# R4 AMD64 enterprise bundle

This directory defines the fresh-install enterprise runtime. It contains no
synthetic identity/model endpoints, build dependencies, company credentials or
legacy volumes. Use the [Chinese deployment guide](../../docs/guides/R4_AMD64_ENTERPRISE_DEPLOYMENT_ZH.md).

Build from a clean pinned upstream and the current verified Node 24 R4 UI artifact:

```sh
python3 tools/release/build_enterprise.py
# Export the public CA bundle from the exact Nginx image recorded in build.json.
# This is automated by the package assembler when the cache file is absent.
python3 tools/release/package_enterprise.py assemble
python3 tools/release/package_enterprise.py export
python3 tools/release/verify_archive.py
python3 tools/release/smoke_enterprise.py
python3 tools/release/verify_restore.py
# The current ARM-host diagnostic additionally records the known failed AMD64
# bin/check-database result, probe_bm25.py results and native ARM64 controls.
# collect_enterprise.py is deliberately strict about these unresolved-check inputs.
python3 tools/release/collect_enterprise.py
python3 tools/release/package_enterprise.py finalize
```

Generated archives live under ignored `images/archives/`. Builds preserve existing
images, data and prior archives. `finalize` requires a completed smoke report for the
same runtime fingerprint, with any unresolved limitations recorded explicitly.
The current package requires target-server BM25 preflight because the AMD64
image aborts on a minimal keyword query under this host emulator. Run-time instance secrets must be outside the bundle, as described in
the guide. The server only needs Docker/Compose and Python 3.10+.

Images are built with the current worktree, not only HEAD. Release metadata records
that fact and the gateway/UI content fingerprints. Go 1.26 and Node 24 remain build
requirements. The offline builder uses cached AMD64 dependencies, a native Go
cross-compile, and the already tested architecture-independent UI assets.

The current acceptance collector is specific to this release's diagnosed BM25
failure. It expects `database-preflight.json`, `bm25-diagnostic.json`, and
`bm25-diagnostic-arm64.json` under `.local/enterprise-amd64/`. Obtain the first
with the packaged `bin/check-database --output ...`; run `probe_bm25.py` for the
other two (`--platform linux/arm64 --image <cached ARM64 reference>` for the
control). Record a different target-server outcome separately; do not copy a
historical report or relax this collector to imply hybrid retrieval passed.
