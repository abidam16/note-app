CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('invitation', 'comment')),
    event_id UUID NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    read_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS notifications_user_type_event_idx
    ON notifications (user_id, type, event_id);

CREATE INDEX IF NOT EXISTS notifications_user_created_idx
    ON notifications (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS notifications_user_unread_idx
    ON notifications (user_id, read_at, created_at DESC)
    WHERE read_at IS NULL;
