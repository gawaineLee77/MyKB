-- Removing progress after external writes could repeat provisioning. Restore
-- the product database and its secret files together instead of erasing state.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.enterprise_installation WHERE stage <> 'new') THEN
        RAISE EXCEPTION 'enterprise installation has external state; use coordinated restore';
    END IF;
END $$;
DROP TABLE mindcreek.enterprise_installation;
