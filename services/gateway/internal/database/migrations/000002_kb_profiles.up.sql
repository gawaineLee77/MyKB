CREATE TABLE mindcreek.kb_profiles (
    upstream_kb_id varchar(128) PRIMARY KEY,
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    owner_user_id varchar(128) NOT NULL,
    product_mode varchar(32) NOT NULL CHECK (product_mode IN ('personal_notes', 'rag', 'ontology')),
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    access_policy varchar(32) NOT NULL CHECK (access_policy IN ('owner_only', 'upstream')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX kb_profiles_tenant_mode_idx
    ON mindcreek.kb_profiles (tenant_id, product_mode);

CREATE INDEX kb_profiles_owner_mode_idx
    ON mindcreek.kb_profiles (owner_user_id, product_mode);
