CREATE TABLE mindcreek.enterprise_installation (
    id smallint PRIMARY KEY CHECK (id = 1),
    stage text NOT NULL CHECK (stage IN ('new','registering','admin_registered','account_ready','space_creating','space_ready','key_creating','ready')),
    admin_email text NOT NULL,
    admin_user_id text NOT NULL DEFAULT '',
    default_tenant_id bigint CHECK (default_tenant_id IS NULL OR default_tenant_id > 0),
    member_key_id text NOT NULL DEFAULT '',
    member_key_ref text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (stage <> 'ready' OR (admin_user_id <> '' AND default_tenant_id IS NOT NULL AND member_key_id <> '' AND member_key_ref <> ''))
);
