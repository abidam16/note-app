CREATE TABLE IF NOT EXISTS page_comment_thread_events (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL REFERENCES page_comment_threads(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    message_id UUID REFERENCES page_comment_messages(id) ON DELETE SET NULL,
    from_thread_state TEXT,
    to_thread_state TEXT,
    from_anchor_state TEXT,
    to_anchor_state TEXT,
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT page_comment_thread_events_type_check CHECK (event_type IN ('created', 'replied', 'resolved', 'reopened', 'anchor_state_changed'))
);

CREATE INDEX IF NOT EXISTS page_comment_thread_events_thread_created_idx
    ON page_comment_thread_events (thread_id, created_at ASC, id ASC);
