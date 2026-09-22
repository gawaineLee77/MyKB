DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.employee_onboarding) THEN
        RAISE EXCEPTION 'employee onboarding has external state; use coordinated restore';
    END IF;
END $$;
DROP TABLE mindcreek.employee_onboarding;
