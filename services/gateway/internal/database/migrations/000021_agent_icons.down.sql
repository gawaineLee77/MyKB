-- Agent avatar references live upstream. Retain referenced images rather than
-- silently breaking existing Agents when rolling the product schema back.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.agent_icons) THEN
        RAISE EXCEPTION 'agent_icons contains uploaded images; preserve or explicitly migrate them before rollback';
    END IF;
END $$;
DROP TABLE IF EXISTS mindcreek.agent_icons;
