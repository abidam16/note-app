ALTER TABLE page_comment_threads
    ADD COLUMN IF NOT EXISTS resolve_note TEXT,
    ADD COLUMN IF NOT EXISTS reopen_reason TEXT;
