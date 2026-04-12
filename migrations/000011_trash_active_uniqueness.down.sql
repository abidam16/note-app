DROP INDEX IF EXISTS trash_items_active_page_unique_idx;

DELETE FROM trash_items
WHERE restored_at IS NOT NULL;

ALTER TABLE trash_items
    ADD CONSTRAINT trash_items_page_id_key UNIQUE (page_id);
