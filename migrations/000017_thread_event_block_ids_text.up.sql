ALTER TABLE page_comment_thread_events
    ALTER COLUMN from_block_id TYPE TEXT USING from_block_id::text,
    ALTER COLUMN to_block_id TYPE TEXT USING to_block_id::text;
