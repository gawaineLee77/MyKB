CREATE TABLE IF NOT EXISTS mindcreek.agent_icons (
    id text PRIMARY KEY CHECK (id ~ '^[a-f0-9]{32}$'),
    tenant_id bigint NOT NULL CHECK (tenant_id > 0),
    user_id text NOT NULL CHECK (user_id <> ''),
    png bytea NOT NULL CHECK (octet_length(png) BETWEEN 1 AND 2097152),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_icons_owner ON mindcreek.agent_icons(tenant_id, user_id);
