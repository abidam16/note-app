DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (
            SELECT workspace_id, parent_id, LOWER(TRIM(name)) AS normalized_name
            FROM folders
            GROUP BY workspace_id, parent_id, LOWER(TRIM(name))
            HAVING COUNT(*) > 1
        ) duplicate_groups
    ) THEN
        RAISE EXCEPTION 'folder sibling-name uniqueness preflight failed: duplicate sibling folder names exist; run "go run ./cmd/migrate -preflight folder-sibling-uniqueness" before applying migration 000009';
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS folders_workspace_root_name_unique_idx
    ON folders (workspace_id, LOWER(TRIM(name)))
    WHERE parent_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS folders_workspace_parent_name_unique_idx
    ON folders (workspace_id, parent_id, LOWER(TRIM(name)))
    WHERE parent_id IS NOT NULL;
