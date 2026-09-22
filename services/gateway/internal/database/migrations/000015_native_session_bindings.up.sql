CREATE TABLE mindcreek.native_session_bindings (
    session_id varchar(128) PRIMARY KEY,
    principal_kind varchar(16) NOT NULL CHECK (principal_kind IN ('human', 'api_key')),
    principal_id varchar(512) NOT NULL CHECK (length(principal_id) > 0),
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    knowledge_base_ids jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(knowledge_base_ids) = 'array' AND jsonb_array_length(knowledge_base_ids) <= 256),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX native_session_actor ON mindcreek.native_session_bindings (tenant_id, principal_kind, principal_id);

CREATE TABLE mindcreek.native_access_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    principal_kind varchar(16) NOT NULL CHECK (principal_kind IN ('human', 'api_key')),
    principal_id varchar(512) NOT NULL,
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    operation varchar(128) NOT NULL,
    outcome varchar(16) NOT NULL CHECK (outcome IN ('allowed', 'denied', 'failure')),
    error_code varchar(128) NOT NULL DEFAULT '',
    knowledge_base_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(knowledge_base_ids) = 'array'),
    correlation_id varchar(128) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX native_access_actor_time ON mindcreek.native_access_events (tenant_id, principal_kind, principal_id, created_at DESC);

-- Deliberately no copy from session_kb_scopes or old product business tables.
