ALTER TABLE page_drafts
    ADD COLUMN IF NOT EXISTS search_body TEXT NOT NULL DEFAULT '';

UPDATE page_drafts
SET search_body = content::text
WHERE search_body = '';

CREATE INDEX IF NOT EXISTS pages_title_search_idx
    ON pages USING GIN (to_tsvector('simple', COALESCE(title, '')));

CREATE INDEX IF NOT EXISTS page_drafts_search_body_idx
    ON page_drafts USING GIN (to_tsvector('simple', COALESCE(search_body, '')));
