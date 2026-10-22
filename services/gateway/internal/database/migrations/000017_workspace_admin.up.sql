-- Product-owned credentials and attribution; no upstream schema changes.
CREATE TABLE mindcreek.workspace_admin_delegations (
    tenant_id bigint PRIMARY KEY CHECK (tenant_id > 0),
    owner_user_id varchar(128) NOT NULL,
    session_cipher bytea NOT NULL,
    member_key_id bigint NOT NULL DEFAULT 0,
    member_key_cipher bytea NOT NULL DEFAULT ''::bytea,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE mindcreek.workspace_api_key_creator_generations (
    user_id varchar(128) PRIMARY KEY,
    generation bigint NOT NULL CHECK (generation > 0)
);
CREATE TABLE mindcreek.workspace_api_key_owners (
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    key_id bigint NOT NULL CHECK (key_id > 0),
    creator_user_id varchar(128) NOT NULL,
    required_role varchar(16) NOT NULL CHECK (required_role IN ('owner','admin')),
    fingerprint char(64) NOT NULL UNIQUE,
    disabled boolean NOT NULL DEFAULT false,
    creator_generation bigint NOT NULL DEFAULT 0 CHECK (creator_generation >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,key_id)
);
CREATE INDEX workspace_api_key_creators ON mindcreek.workspace_api_key_owners(tenant_id,creator_user_id);
CREATE TABLE mindcreek.workspace_api_key_pending (
    tenant_id bigint NOT NULL,
    marker varchar(128) NOT NULL,
    key_id bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,marker)
);
CREATE TABLE mindcreek.workspace_admin_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id bigint NOT NULL,
    actor_user_id varchar(128) NOT NULL,
    action varchar(80) NOT NULL,
    target varchar(128) NOT NULL DEFAULT '',
    outcome varchar(32) NOT NULL,
    correlation_id varchar(128) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workspace_admin_audit_tenant_time ON mindcreek.workspace_admin_audit(tenant_id,created_at DESC);
