ALTER TABLE notifications
    ADD COLUMN actor_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN title TEXT,
    ADD COLUMN content TEXT,
    ADD COLUMN is_read BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN actionable BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN action_kind TEXT NULL,
    ADD COLUMN resource_type TEXT NULL,
    ADD COLUMN resource_id UUID NULL,
    ADD COLUMN payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN updated_at TIMESTAMPTZ;

UPDATE notifications
SET title = CASE
        WHEN type = 'invitation' THEN 'Workspace invitation'
        WHEN type = 'comment' THEN 'Comment activity'
        ELSE 'Notification'
    END,
    content = message,
    is_read = (read_at IS NOT NULL),
    actionable = FALSE,
    action_kind = NULL,
    resource_type = CASE
        WHEN type = 'invitation' THEN 'invitation'
        ELSE NULL
    END,
    resource_id = CASE
        WHEN type = 'invitation' THEN event_id
        ELSE NULL
    END,
    payload = '{}'::jsonb,
    updated_at = COALESCE(read_at, created_at)
WHERE title IS NULL
   OR content IS NULL
   OR updated_at IS NULL;

ALTER TABLE notifications
    ALTER COLUMN title SET NOT NULL,
    ALTER COLUMN content SET NOT NULL,
    ALTER COLUMN updated_at SET NOT NULL;

ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_check
        CHECK (type IN ('invitation', 'comment', 'mention')),
    ADD CONSTRAINT notifications_is_read_consistency_check
        CHECK (
            (is_read = FALSE AND read_at IS NULL)
            OR
            (is_read = TRUE AND read_at IS NOT NULL)
        ),
    ADD CONSTRAINT notifications_action_kind_consistency_check
        CHECK (
            action_kind IS NULL
            OR
            (actionable = TRUE AND action_kind = 'invitation_response')
        ),
    ADD CONSTRAINT notifications_resource_link_consistency_check
        CHECK (
            (resource_type IS NULL AND resource_id IS NULL)
            OR
            (
                resource_type IN ('invitation', 'page_comment', 'thread', 'thread_message')
                AND resource_id IS NOT NULL
            )
        );

DROP INDEX IF EXISTS notifications_user_unread_idx;

CREATE INDEX IF NOT EXISTS notifications_user_unread_idx
    ON notifications (user_id, created_at DESC, id DESC)
    WHERE is_read = FALSE;

CREATE INDEX IF NOT EXISTS notifications_user_type_created_idx
    ON notifications (user_id, type, created_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS notifications_user_invitation_resource_idx
    ON notifications (user_id, resource_id)
    WHERE type = 'invitation' AND resource_type = 'invitation';
