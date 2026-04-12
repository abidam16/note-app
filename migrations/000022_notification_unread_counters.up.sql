CREATE TABLE notification_unread_counters (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    unread_count BIGINT NOT NULL DEFAULT 0 CHECK (unread_count >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO notification_unread_counters (user_id, unread_count, created_at, updated_at)
SELECT
    user_id,
    COUNT(*)::BIGINT AS unread_count,
    MIN(created_at) AS created_at,
    MAX(updated_at) AS updated_at
FROM notifications
WHERE is_read = FALSE
GROUP BY user_id
ON CONFLICT (user_id) DO UPDATE
SET unread_count = EXCLUDED.unread_count,
    updated_at = EXCLUDED.updated_at;
