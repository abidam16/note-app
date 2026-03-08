CREATE TABLE IF NOT EXISTS page_comments (
    id UUID PRIMARY KEY,
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    resolved_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS page_comments_page_created_idx
    ON page_comments (page_id, created_at ASC, id ASC);
