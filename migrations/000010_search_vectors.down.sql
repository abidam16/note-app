DROP INDEX IF EXISTS page_drafts_search_body_vector_idx;
DROP INDEX IF EXISTS pages_title_search_vector_idx;

ALTER TABLE page_drafts
    DROP COLUMN IF EXISTS search_body_vector;

ALTER TABLE pages
    DROP COLUMN IF EXISTS title_search;

CREATE INDEX IF NOT EXISTS pages_title_search_idx
    ON pages USING GIN (to_tsvector('simple', COALESCE(title, '')));

CREATE INDEX IF NOT EXISTS page_drafts_search_body_idx
    ON page_drafts USING GIN (to_tsvector('simple', COALESCE(search_body, '')));
