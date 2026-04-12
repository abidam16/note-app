ALTER TABLE page_comment_thread_events
    ADD COLUMN IF NOT EXISTS from_block_id TEXT,
    ADD COLUMN IF NOT EXISTS to_block_id TEXT;

ALTER TABLE page_comment_thread_events
    DROP CONSTRAINT IF EXISTS page_comment_thread_events_type_check;

ALTER TABLE page_comment_thread_events
    ADD CONSTRAINT page_comment_thread_events_type_check
    CHECK (event_type IN ('created', 'replied', 'resolved', 'reopened', 'anchor_state_changed', 'anchor_recovered'));
