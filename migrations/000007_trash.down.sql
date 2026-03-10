DROP INDEX IF EXISTS pages_workspace_active_idx;
DROP INDEX IF EXISTS trash_items_workspace_deleted_idx;
DROP TABLE IF EXISTS trash_items;
ALTER TABLE pages
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;
