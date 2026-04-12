ALTER TABLE page_comment_thread_events
    ADD COLUMN IF NOT EXISTS reason TEXT;
