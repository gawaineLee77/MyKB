CREATE TABLE mindcreek.corporate_identities (
    issuer varchar(2048) NOT NULL,
    subject varchar(1024) NOT NULL,
    broker_subject varchar(64) NOT NULL UNIQUE,
    upstream_email varchar(320) NOT NULL UNIQUE,
    corporate_email varchar(320) NOT NULL,
    username varchar(160) NOT NULL,
    display_name varchar(320) NOT NULL DEFAULT '',
    groups_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    status varchar(16) NOT NULL DEFAULT 'active',
    local_user_id varchar(512),
    local_tenant_id bigint,
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    suspended_at timestamptz,
    PRIMARY KEY (issuer, subject),
    CONSTRAINT corporate_identity_groups_array CHECK (jsonb_typeof(groups_json) = 'array'),
    CONSTRAINT corporate_identity_status CHECK (status IN ('active', 'suspended')),
    CONSTRAINT corporate_identity_tenant_positive CHECK (local_tenant_id IS NULL OR local_tenant_id > 0),
    CONSTRAINT corporate_identity_time_order CHECK (last_seen_at >= first_seen_at),
    CONSTRAINT corporate_identity_suspension_state CHECK (
        (status = 'active' AND suspended_at IS NULL) OR
        (status = 'suspended' AND suspended_at IS NOT NULL)
    )
);

CREATE INDEX corporate_identities_upstream_email
    ON mindcreek.corporate_identities (upstream_email);

CREATE INDEX corporate_identities_local_user
    ON mindcreek.corporate_identities (local_user_id)
    WHERE local_user_id IS NOT NULL;

CREATE TABLE mindcreek.identity_audit_events (
    id varchar(36) PRIMARY KEY,
    issuer varchar(2048) NOT NULL,
    subject_hash varchar(64) NOT NULL,
    local_user_id varchar(512),
    action varchar(64) NOT NULL,
    outcome varchar(16) NOT NULL,
    error_code varchar(128),
    correlation_id varchar(128) NOT NULL,
    source_ip_hash varchar(64),
    created_at timestamptz NOT NULL,
    CONSTRAINT identity_audit_outcome CHECK (outcome IN ('success', 'denied', 'failure'))
);

CREATE INDEX identity_audit_subject_time
    ON mindcreek.identity_audit_events (subject_hash, created_at DESC);

CREATE INDEX identity_audit_correlation
    ON mindcreek.identity_audit_events (correlation_id);
