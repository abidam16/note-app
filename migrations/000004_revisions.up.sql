CREATE TABLE IF NOT EXISTS revisions (
    id UUID PRIMARY KEY,
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    label TEXT,
    note TEXT,
    content JSONB NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS revisions_page_created_idx
    ON revisions (page_id, created_at DESC);
