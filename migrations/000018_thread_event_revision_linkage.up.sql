ALTER TABLE page_comment_thread_events
    ADD COLUMN IF NOT EXISTS revision_id UUID REFERENCES revisions(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS page_comment_thread_events_revision_idx
    ON page_comment_thread_events (revision_id)
    WHERE revision_id IS NOT NULL;
