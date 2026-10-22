DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.kb_resource_bindings) THEN
        RAISE EXCEPTION 'kb_resource_bindings contains reviewed ownership evidence; preserve or explicitly migrate it before rollback';
    END IF;
END $$;
DROP TABLE IF EXISTS mindcreek.kb_resource_bindings;
