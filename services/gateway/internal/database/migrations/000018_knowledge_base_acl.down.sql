-- Tombstones and generations are authorization data, including for deleted KBs.
-- Losing them could reactivate historical creator or collaborator access.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mindcreek.kb_policies)
       OR EXISTS (SELECT 1 FROM mindcreek.kb_member_permissions)
       OR EXISTS (SELECT 1 FROM mindcreek.kb_acl_generations)
       OR EXISTS (SELECT 1 FROM mindcreek.kb_acl_events)
       OR EXISTS (SELECT 1 FROM mindcreek.kb_creation_operations) THEN
        RAISE EXCEPTION 'knowledge-base ACL metadata must be retained; rollback requires an empty ACL deployment';
    END IF;
END $$;
DROP TABLE mindcreek.kb_creation_operations;
DROP TABLE mindcreek.kb_acl_events;
DROP TABLE mindcreek.kb_member_permissions;
DROP TABLE mindcreek.kb_policies;
DROP TABLE mindcreek.kb_acl_generations;
