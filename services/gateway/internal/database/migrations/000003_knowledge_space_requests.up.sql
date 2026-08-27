CREATE TABLE mindcreek.knowledge_space_requests (
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    owner_user_id varchar(128) NOT NULL,
    idempotency_key varchar(128) NOT NULL,
    request_hash char(64) NOT NULL,
    upstream_kb_id varchar(128) NOT NULL UNIQUE,
    product_mode varchar(32) NOT NULL CHECK (product_mode IN ('personal_notes', 'rag')),
    index_profile varchar(32) NOT NULL CHECK (index_profile IN ('notes_plain', 'plain')),
    status varchar(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed')),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, owner_user_id, idempotency_key)
);

CREATE INDEX knowledge_space_requests_status_idx
    ON mindcreek.knowledge_space_requests (status, updated_at);
