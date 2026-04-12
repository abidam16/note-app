ALTER TABLE trash_items
    DROP CONSTRAINT IF EXISTS trash_items_page_id_key;

DROP INDEX IF EXISTS trash_items_active_page_unique_idx;

CREATE UNIQUE INDEX trash_items_active_page_unique_idx
    ON trash_items (page_id)
    WHERE restored_at IS NULL;
