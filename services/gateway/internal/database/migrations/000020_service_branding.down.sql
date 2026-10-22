-- Keep configured branding and its audit history during application rollback.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.service_branding WHERE revision > 1)
       OR EXISTS (SELECT 1 FROM mindcreek.service_branding_events) THEN
        RAISE EXCEPTION 'service branding contains configuration; preserve it during rollback';
    END IF;
END $$;
DROP TABLE mindcreek.service_branding_events;
DROP TABLE mindcreek.service_branding;
