-- NPC upserts match on (sheet_id, sheet_name) so sheetssync can refresh a
-- given tab's stats without creating duplicates. Also lets FindNpcBySheetName
-- use the index for /npc lookups.

CREATE UNIQUE INDEX IF NOT EXISTS npcs_sheet_id_name_unique
    ON npcs (sheet_id, sheet_name);

-- Character upserts key on (user_id, name) so one user can have several
-- characters with different names, but re-syncing the same sheet doesn't
-- create a duplicate.
CREATE UNIQUE INDEX IF NOT EXISTS characters_user_id_name_unique
    ON characters (user_id, name);
