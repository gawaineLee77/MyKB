"""Explicit historical releases exercised by the current offline package."""

UPGRADE_BASELINES = {
    'upgrade-r4': 'r4-20260916-118b8af3',
    'upgrade-1009': 'current-20261009-a86981ad',
    'upgrade-1019': 'current-20261019-d3fafdaf',
}
SOURCE_RELEASES = {
    'r4-20260916-118b8af3': 15,
    'current-20261009-a86981ad': 16,
    'current-20261019-d3fafdaf': 21,
}


def source_has_knowledge_acl(release):
    return SOURCE_RELEASES[release] >= 18


def upgrade_key_check(release):
    return ('upgrade_existing_machine_key_scope_preserved' if source_has_knowledge_acl(release)
            else 'upgrade_legacy_machine_key_requires_explicit_mapping')
