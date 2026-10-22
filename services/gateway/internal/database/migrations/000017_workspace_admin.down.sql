-- Refuse to erase revocation/attribution or live delegated credentials. A
-- destructive rollback would turn managed keys into untracked legacy keys.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.workspace_api_key_owners)
       OR EXISTS (SELECT 1 FROM mindcreek.workspace_admin_delegations)
       OR EXISTS (SELECT 1 FROM mindcreek.workspace_api_key_pending)
       OR EXISTS (SELECT 1 FROM mindcreek.workspace_api_key_creator_generations) THEN
        RAISE EXCEPTION 'workspace administration metadata must be retained while managed keys or delegations exist';
    END IF;
END $$;
DROP TABLE mindcreek.workspace_admin_audit;
DROP TABLE mindcreek.workspace_api_key_pending;
DROP TABLE mindcreek.workspace_api_key_owners;
DROP TABLE mindcreek.workspace_api_key_creator_generations;
DROP TABLE mindcreek.workspace_admin_delegations;
