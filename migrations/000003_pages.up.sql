CREATE TABLE IF NOT EXISTS pages (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    folder_id UUID REFERENCES folders(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS pages_workspace_created_idx
    ON pages (workspace_id, created_at);

CREATE INDEX IF NOT EXISTS pages_folder_idx
    ON pages (folder_id);

CREATE TABLE IF NOT EXISTS page_drafts (
    page_id UUID PRIMARY KEY REFERENCES pages(id) ON DELETE CASCADE,
    content JSONB NOT NULL,
    last_edited_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
