ALTER TABLE mindcreek.kb_profiles
    ADD COLUMN index_profile varchar(32),
    ADD COLUMN index_profile_version integer,
    ADD COLUMN effective_config jsonb;

UPDATE mindcreek.kb_profiles
SET index_profile = CASE product_mode
        WHEN 'personal_notes' THEN 'notes_plain'
        WHEN 'rag' THEN 'plain'
        ELSE 'ontology_draft'
    END,
    index_profile_version = 1,
    effective_config = CASE product_mode
        WHEN 'personal_notes' THEN '{"profile_id":"notes_plain","profile_version":1,"legacy_backfill":true}'::jsonb
        WHEN 'rag' THEN '{"profile_id":"plain","profile_version":1,"legacy_backfill":true}'::jsonb
        ELSE '{"profile_id":"ontology_draft","profile_version":1,"legacy_backfill":true}'::jsonb
    END;

ALTER TABLE mindcreek.kb_profiles
    ALTER COLUMN index_profile SET NOT NULL,
    ALTER COLUMN index_profile_version SET NOT NULL,
    ALTER COLUMN effective_config SET NOT NULL,
    ADD CONSTRAINT kb_profiles_index_profile_version_positive CHECK (index_profile_version > 0),
    ADD CONSTRAINT kb_profiles_mode_index_profile CHECK (
        (product_mode = 'personal_notes' AND index_profile = 'notes_plain') OR
        (product_mode = 'rag' AND index_profile = 'plain') OR
        (product_mode = 'ontology' AND index_profile = 'ontology_draft')
    );
