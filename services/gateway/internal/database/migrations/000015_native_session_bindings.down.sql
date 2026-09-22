DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.native_session_bindings LIMIT 1)
       OR EXISTS (SELECT 1 FROM mindcreek.native_access_events LIMIT 1) THEN
        RAISE EXCEPTION 'Refusing to remove populated R3 session bindings or audit records';
    END IF;
END $$;
DROP TABLE mindcreek.native_access_events;
DROP TABLE mindcreek.native_session_bindings;
