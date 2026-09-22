DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.assistant_sessions LIMIT 1) THEN
        RAISE EXCEPTION 'Refusing to remove populated employee channel session bindings';
    END IF;
END $$;
DROP TABLE mindcreek.assistant_rate_windows;
DROP TABLE mindcreek.assistant_sessions;
