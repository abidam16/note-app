ALTER TABLE pages
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE RESTRICT;

CREATE TABLE IF NOT EXISTS trash_items (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    page_id UUID NOT NULL UNIQUE REFERENCES pages(id) ON DELETE CASCADE,
    page_title TEXT NOT NULL,
    deleted_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    deleted_at TIMESTAMPTZ NOT NULL,
    restored_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    restored_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS trash_items_workspace_deleted_idx
    ON trash_items (workspace_id, deleted_at DESC, id ASC)
    WHERE restored_at IS NULL;

CREATE INDEX IF NOT EXISTS pages_workspace_active_idx
    ON pages (workspace_id, updated_at DESC)
    WHERE deleted_at IS NULL;
