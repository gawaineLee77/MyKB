-- Explicit installation-operator review supplements immutable native parser
-- provenance. No upstream rows are modified and no body-derived associations
-- are backfilled automatically.
CREATE TABLE IF NOT EXISTS mindcreek.kb_resource_bindings (
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    resource_id text NOT NULL CHECK (resource_id <> ''),
    resource_fingerprint text NOT NULL CHECK (resource_fingerprint ~ '^[a-f0-9]{64}$'),
    knowledge_id text NOT NULL CHECK (knowledge_id <> ''),
    reason text NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, resource_id, resource_fingerprint, knowledge_id)
);
CREATE INDEX IF NOT EXISTS kb_resource_bindings_knowledge
    ON mindcreek.kb_resource_bindings(tenant_id, knowledge_id);
