ALTER TABLE page_comment_threads
    DROP COLUMN IF EXISTS reopen_reason,
    DROP COLUMN IF EXISTS resolve_note;
