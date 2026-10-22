-- Product-owned ACL; never alters native membership or KB records.
CREATE TABLE mindcreek.kb_acl_generations (
    tenant_id bigint NOT NULL CHECK (tenant_id >= 0),
    user_id varchar(128) NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    PRIMARY KEY (tenant_id,user_id)
);
CREATE TABLE mindcreek.kb_policies (
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    kb_id varchar(128) NOT NULL,
    creator_user_id varchar(128) NOT NULL DEFAULT '',
    creator_revoked boolean NOT NULL DEFAULT false,
    creator_member_generation bigint NOT NULL DEFAULT 0,
    creator_identity_generation bigint NOT NULL DEFAULT 0,
    visibility varchar(16) NOT NULL DEFAULT 'restricted' CHECK (visibility IN ('restricted','workspace')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    deleted boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,kb_id)
);
CREATE INDEX kb_policies_creator ON mindcreek.kb_policies(tenant_id,creator_user_id);
CREATE TABLE mindcreek.kb_member_permissions (
    tenant_id bigint NOT NULL,
    kb_id varchar(128) NOT NULL,
    user_id varchar(128) NOT NULL,
    visible boolean NOT NULL,
    operations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(operations)='array'),
    member_generation bigint NOT NULL DEFAULT 0,
    identity_generation bigint NOT NULL DEFAULT 0,
    granted_by varchar(128) NOT NULL,
    revision bigint NOT NULL,
    PRIMARY KEY (tenant_id,kb_id,user_id),
    FOREIGN KEY (tenant_id,kb_id) REFERENCES mindcreek.kb_policies(tenant_id,kb_id),
    CHECK (visible OR operations='[]'::jsonb)
);
CREATE INDEX kb_member_permissions_subject ON mindcreek.kb_member_permissions(tenant_id,user_id);
CREATE TABLE mindcreek.kb_acl_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id bigint NOT NULL,
    kb_id varchar(128) NOT NULL,
    actor_user_id varchar(128) NOT NULL,
    action varchar(80) NOT NULL,
    target varchar(128) NOT NULL DEFAULT '',
    revision bigint NOT NULL,
    before_state jsonb NOT NULL,
    after_state jsonb NOT NULL,
    outcome varchar(32) NOT NULL,
    request_id varchar(128) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX kb_acl_events_kb ON mindcreek.kb_acl_events(tenant_id,kb_id,id DESC);
CREATE TABLE mindcreek.kb_creation_operations (
    id char(32) PRIMARY KEY,
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    actor_user_id varchar(128) NOT NULL,
    key_hash char(64) NOT NULL,
    request_hash char(64) NOT NULL,
    baseline_kb_ids jsonb NOT NULL CHECK (jsonb_typeof(baseline_kb_ids)='array'),
    state varchar(16) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','completed')),
    kb_id varchar(128) NOT NULL DEFAULT '',
    request_id varchar(128) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id,actor_user_id,key_hash),
    CHECK ((state='pending' AND kb_id='') OR (state='completed' AND kb_id<>''))
);
CREATE INDEX kb_creation_pending_actor ON mindcreek.kb_creation_operations(tenant_id,actor_user_id) WHERE state='pending';
CREATE INDEX kb_creation_completed_kb ON mindcreek.kb_creation_operations(tenant_id,kb_id) WHERE state='completed';
