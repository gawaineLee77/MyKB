-- Knowledge execution is a separate, explicitly authorized delegation purpose.
CREATE TABLE mindcreek.kb_execution_delegations (
    tenant_id bigint PRIMARY KEY CHECK (tenant_id > 0),
    owner_user_id varchar(128) NOT NULL,
    session_cipher bytea NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE mindcreek.kb_execution_keys (
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    kb_id varchar(128) NOT NULL,
    key_id bigint NOT NULL CHECK (key_id > 0),
    key_cipher bytea NOT NULL,
    PRIMARY KEY (tenant_id,kb_id)
);
CREATE TABLE mindcreek.kb_machine_permissions (
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    key_id bigint NOT NULL CHECK (key_id > 0),
    fingerprint char(64) NOT NULL UNIQUE,
    knowledge_base_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(knowledge_base_ids)='array'),
    operations jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(operations)='array'),
    all_knowledge_bases boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,key_id)
);
