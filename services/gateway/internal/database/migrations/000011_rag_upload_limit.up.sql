UPDATE mindcreek.kb_profiles
SET effective_config = jsonb_set(
        effective_config,
        '{limits}',
        COALESCE(effective_config -> 'limits', '{}'::jsonb) || '{"max_file_bytes":209715200}'::jsonb,
        true
    ),
    updated_at = now()
WHERE product_mode = 'rag'
  AND index_profile = 'plain'
  AND (
      effective_config #> '{limits,max_file_bytes}' IS NULL
      OR effective_config #> '{limits,max_file_bytes}' = '52428800'::jsonb
  );
