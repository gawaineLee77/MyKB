CREATE TABLE mindcreek.kb_access_grants (
    id varchar(36) PRIMARY KEY,
    knowledge_base_id varchar(36) NOT NULL,
    subject_type varchar(32) NOT NULL,
    subject_id varchar(36) NOT NULL,
    permission varchar(16) NOT NULL,
    granted_by varchar(36) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    revoked_at timestamptz,
    revision bigint NOT NULL DEFAULT 1,
    last_audit_correlation_id varchar(128) NOT NULL,
    CONSTRAINT kb_access_grants_subject_type CHECK (subject_type IN ('user', 'group', 'workspace')),
    CONSTRAINT kb_access_grants_permission CHECK (permission IN ('viewer', 'editor')),
    CONSTRAINT kb_access_grants_revision_positive CHECK (revision > 0),
    CONSTRAINT kb_access_grants_expiry_after_creation CHECK (expires_at IS NULL OR expires_at > created_at),
    CONSTRAINT kb_access_grants_revocation_after_creation CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE UNIQUE INDEX kb_access_grants_active_subject_unique
    ON mindcreek.kb_access_grants (knowledge_base_id, subject_type, subject_id)
    WHERE revoked_at IS NULL;

CREATE INDEX kb_access_grants_kb_active
    ON mindcreek.kb_access_grants (knowledge_base_id, permission)
    WHERE revoked_at IS NULL;

CREATE INDEX kb_access_grants_subject_active
    ON mindcreek.kb_access_grants (subject_type, subject_id, knowledge_base_id)
    WHERE revoked_at IS NULL;

CREATE INDEX kb_access_grants_expiry
    ON mindcreek.kb_access_grants (expires_at)
    WHERE revoked_at IS NULL AND expires_at IS NOT NULL;
