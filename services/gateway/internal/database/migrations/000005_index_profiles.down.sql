ALTER TABLE mindcreek.kb_profiles
    DROP CONSTRAINT IF EXISTS kb_profiles_mode_index_profile,
    DROP CONSTRAINT IF EXISTS kb_profiles_index_profile_version_positive,
    DROP COLUMN IF EXISTS effective_config,
    DROP COLUMN IF EXISTS index_profile_version,
    DROP COLUMN IF EXISTS index_profile;
