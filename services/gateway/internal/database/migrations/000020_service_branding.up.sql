CREATE TABLE mindcreek.service_branding (
    id smallint PRIMARY KEY CHECK (id = 1),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    logo bytea NOT NULL DEFAULT ''::bytea CHECK (octet_length(logo) <= 2097152),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_by text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO mindcreek.service_branding (id,name) VALUES (1,'小纳');
CREATE TABLE mindcreek.service_branding_events (
    id bigserial PRIMARY KEY,
    actor_user_id text NOT NULL,
    request_id text NOT NULL,
    before_state jsonb NOT NULL,
    after_state jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
