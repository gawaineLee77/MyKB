CREATE TABLE mindcreek.employee_onboarding (
    broker_subject varchar(64) PRIMARY KEY REFERENCES mindcreek.corporate_identities(broker_subject),
    local_user_id text NOT NULL UNIQUE,
    default_tenant_id bigint NOT NULL CHECK (default_tenant_id > 0),
    state text NOT NULL CHECK (state IN ('pending','joining','ready','failed')),
    outcome_unknown boolean NOT NULL DEFAULT false,
    completed_at timestamptz,
    error_code text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (state <> 'ready' OR completed_at IS NOT NULL)
);
