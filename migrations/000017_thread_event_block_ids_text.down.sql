ALTER TABLE page_comment_thread_events
    ALTER COLUMN from_block_id TYPE UUID USING NULLIF(from_block_id, '')::uuid,
    ALTER COLUMN to_block_id TYPE UUID USING NULLIF(to_block_id, '')::uuid;
