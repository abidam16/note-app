CREATE TABLE IF NOT EXISTS page_comment_threads (
    id UUID PRIMARY KEY,
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    anchor_type TEXT NOT NULL,
    block_id TEXT,
    quoted_text TEXT,
    quoted_block_text TEXT NOT NULL,
    thread_state TEXT NOT NULL,
    anchor_state TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL,
    resolved_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    resolved_at TIMESTAMPTZ,
    reopened_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    reopened_at TIMESTAMPTZ,
    last_activity_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT page_comment_threads_anchor_type_check CHECK (anchor_type IN ('block', 'page_legacy')),
    CONSTRAINT page_comment_threads_thread_state_check CHECK (thread_state IN ('open', 'resolved')),
    CONSTRAINT page_comment_threads_anchor_state_check CHECK (anchor_state IN ('active', 'outdated', 'missing')),
    CONSTRAINT page_comment_threads_block_anchor_check CHECK ((anchor_type <> 'block') OR (block_id IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS page_comment_messages (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL REFERENCES page_comment_threads(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS page_comment_threads_page_activity_idx
    ON page_comment_threads (page_id, thread_state, last_activity_at DESC, id ASC);

CREATE INDEX IF NOT EXISTS page_comment_threads_page_anchor_idx
    ON page_comment_threads (page_id, anchor_state, last_activity_at DESC, id ASC);

CREATE INDEX IF NOT EXISTS page_comment_messages_thread_created_idx
    ON page_comment_messages (thread_id, created_at ASC, id ASC);
