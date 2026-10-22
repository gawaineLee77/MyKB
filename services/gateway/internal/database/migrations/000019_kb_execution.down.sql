DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.kb_execution_delegations)
       OR EXISTS (SELECT 1 FROM mindcreek.kb_execution_keys)
       OR EXISTS (SELECT 1 FROM mindcreek.kb_machine_permissions) THEN
        RAISE EXCEPTION 'Cannot remove live KB execution or machine authorization metadata';
    END IF;
END $$;
DROP TABLE mindcreek.kb_machine_permissions;
DROP TABLE mindcreek.kb_execution_keys;
DROP TABLE mindcreek.kb_execution_delegations;
