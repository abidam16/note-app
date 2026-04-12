ALTER TABLE pages
    ADD COLUMN IF NOT EXISTS title_search tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', COALESCE(title, ''))) STORED;

ALTER TABLE page_drafts
    ADD COLUMN IF NOT EXISTS search_body_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', COALESCE(search_body, ''))) STORED;

DROP INDEX IF EXISTS pages_title_search_idx;
DROP INDEX IF EXISTS page_drafts_search_body_idx;

CREATE INDEX IF NOT EXISTS pages_title_search_vector_idx
    ON pages USING GIN (title_search);

CREATE INDEX IF NOT EXISTS page_drafts_search_body_vector_idx
    ON page_drafts USING GIN (search_body_vector);
