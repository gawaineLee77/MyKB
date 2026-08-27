CREATE TABLE mindcreek.session_kb_scopes (
    session_id varchar(128) NOT NULL,
    knowledge_base_id varchar(36) NOT NULL,
    first_recorded_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    PRIMARY KEY (session_id, knowledge_base_id),
    CONSTRAINT session_kb_scopes_time_order CHECK (last_seen_at >= first_recorded_at)
);

CREATE INDEX session_kb_scopes_kb
    ON mindcreek.session_kb_scopes (knowledge_base_id, session_id);

CREATE TABLE mindcreek.kb_access_audit_events (
    id varchar(36) PRIMARY KEY,
    tenant_id bigint NOT NULL,
    knowledge_base_id varchar(36) NOT NULL,
    actor_user_id varchar(512) NOT NULL,
    action varchar(64) NOT NULL,
    target_type varchar(32) NOT NULL,
    target_id varchar(128) NOT NULL,
    outcome varchar(16) NOT NULL,
    error_code varchar(128),
    correlation_id varchar(128) NOT NULL,
    old_value jsonb,
    new_value jsonb,
    created_at timestamptz NOT NULL,
    CONSTRAINT kb_access_audit_tenant_positive CHECK (tenant_id > 0),
    CONSTRAINT kb_access_audit_outcome CHECK (outcome IN ('success', 'denied', 'failure')),
    CONSTRAINT kb_access_audit_old_object CHECK (old_value IS NULL OR jsonb_typeof(old_value) = 'object'),
    CONSTRAINT kb_access_audit_new_object CHECK (new_value IS NULL OR jsonb_typeof(new_value) = 'object')
);

CREATE INDEX kb_access_audit_kb_time
    ON mindcreek.kb_access_audit_events (knowledge_base_id, created_at DESC);

CREATE INDEX kb_access_audit_actor_time
    ON mindcreek.kb_access_audit_events (actor_user_id, created_at DESC);

CREATE INDEX kb_access_audit_correlation
    ON mindcreek.kb_access_audit_events (correlation_id);
