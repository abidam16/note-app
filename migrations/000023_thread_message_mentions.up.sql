CREATE TABLE page_comment_message_mentions (
    message_id UUID NOT NULL REFERENCES page_comment_messages(id) ON DELETE CASCADE,
    mentioned_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    PRIMARY KEY (message_id, mentioned_user_id)
);

CREATE INDEX page_comment_message_mentions_user_message_idx
    ON page_comment_message_mentions (mentioned_user_id, message_id);
