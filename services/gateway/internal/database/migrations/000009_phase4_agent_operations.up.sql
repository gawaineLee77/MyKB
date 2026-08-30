CREATE TABLE mindcreek.agent_operation_audit_events (
    id varchar(36) PRIMARY KEY,
    tenant_id bigint NOT NULL,
    actor_user_id varchar(512) NOT NULL,
    client_kind varchar(16) NOT NULL,
    operation varchar(64) NOT NULL,
    knowledge_base_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    outcome varchar(16) NOT NULL,
    error_code varchar(128),
    correlation_id varchar(128) NOT NULL,
    duration_ms bigint NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_operation_tenant_positive CHECK (tenant_id > 0),
    CONSTRAINT agent_operation_client_kind CHECK (client_kind IN ('web', 'mcp')),
    CONSTRAINT agent_operation_scope_array CHECK (jsonb_typeof(knowledge_base_ids) = 'array'),
    CONSTRAINT agent_operation_scope_limit CHECK (jsonb_array_length(knowledge_base_ids) <= 64),
    CONSTRAINT agent_operation_outcome CHECK (outcome IN ('success', 'denied', 'failure')),
    CONSTRAINT agent_operation_duration_nonnegative CHECK (duration_ms >= 0)
);

CREATE INDEX agent_operation_actor_time
    ON mindcreek.agent_operation_audit_events (actor_user_id, created_at DESC);

CREATE INDEX agent_operation_correlation
    ON mindcreek.agent_operation_audit_events (correlation_id);
