CREATE TABLE mindcreek.note_revisions (
    upstream_kb_id varchar(128) NOT NULL REFERENCES mindcreek.kb_profiles(upstream_kb_id) ON DELETE CASCADE,
    upstream_note_id varchar(128) NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    title varchar(200) NOT NULL,
    content text NOT NULL CHECK (octet_length(content) <= 65536),
    status varchar(16) NOT NULL CHECK (status IN ('draft', 'publish')),
    operation varchar(16) NOT NULL CHECK (operation IN ('create', 'edit', 'import', 'restore', 'snapshot')),
    restored_from_version integer CHECK (restored_from_version IS NULL OR restored_from_version > 0),
    content_sha256 char(64) NOT NULL,
    actor_user_id varchar(128) NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (upstream_kb_id, upstream_note_id, version)
);

CREATE INDEX note_revisions_latest_idx
    ON mindcreek.note_revisions (upstream_kb_id, upstream_note_id, version DESC);
