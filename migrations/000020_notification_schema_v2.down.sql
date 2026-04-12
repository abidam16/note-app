DROP INDEX IF EXISTS notifications_user_invitation_resource_idx;
DROP INDEX IF EXISTS notifications_user_type_created_idx;
DROP INDEX IF EXISTS notifications_user_unread_idx;

ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS notifications_resource_link_consistency_check,
    DROP CONSTRAINT IF EXISTS notifications_action_kind_consistency_check,
    DROP CONSTRAINT IF EXISTS notifications_is_read_consistency_check,
    DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_check
        CHECK (type IN ('invitation', 'comment'));

ALTER TABLE notifications
    DROP COLUMN IF EXISTS actor_id,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS content,
    DROP COLUMN IF EXISTS is_read,
    DROP COLUMN IF EXISTS actionable,
    DROP COLUMN IF EXISTS action_kind,
    DROP COLUMN IF EXISTS resource_type,
    DROP COLUMN IF EXISTS resource_id,
    DROP COLUMN IF EXISTS payload,
    DROP COLUMN IF EXISTS updated_at;

CREATE INDEX IF NOT EXISTS notifications_user_unread_idx
    ON notifications (user_id, read_at, created_at DESC)
    WHERE read_at IS NULL;
