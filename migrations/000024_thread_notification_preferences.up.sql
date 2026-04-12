CREATE TABLE thread_notification_preferences (
    thread_id UUID NOT NULL REFERENCES page_comment_threads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (thread_id, user_id),
    CONSTRAINT thread_notification_preferences_mode_check CHECK (mode IN ('all', 'mentions_only', 'mute'))
);
