CREATE TABLE IF NOT EXISTS folders (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_id UUID REFERENCES folders(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS folders_workspace_created_idx
    ON folders (workspace_id, created_at);

CREATE INDEX IF NOT EXISTS folders_workspace_parent_idx
    ON folders (workspace_id, parent_id);
