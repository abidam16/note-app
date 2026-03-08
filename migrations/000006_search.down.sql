DROP INDEX IF EXISTS page_drafts_search_body_idx;
DROP INDEX IF EXISTS pages_title_search_idx;
ALTER TABLE page_drafts DROP COLUMN IF EXISTS search_body;
