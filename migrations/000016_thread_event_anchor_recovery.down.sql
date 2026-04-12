ALTER TABLE page_comment_thread_events
    DROP CONSTRAINT IF EXISTS page_comment_thread_events_type_check;

ALTER TABLE page_comment_thread_events
    ADD CONSTRAINT page_comment_thread_events_type_check
    CHECK (event_type IN ('created', 'replied', 'resolved', 'reopened', 'anchor_state_changed'));

ALTER TABLE page_comment_thread_events
    DROP COLUMN IF EXISTS to_block_id,
    DROP COLUMN IF EXISTS from_block_id;
