DROP INDEX IF EXISTS page_comment_thread_events_revision_idx;

ALTER TABLE page_comment_thread_events
    DROP COLUMN IF EXISTS revision_id;
