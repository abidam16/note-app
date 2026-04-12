CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    topic TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 25,
    available_at TIMESTAMPTZ NOT NULL,
    claimed_by TEXT NULL,
    claimed_at TIMESTAMPTZ NULL,
    lease_expires_at TIMESTAMPTZ NULL,
    last_error TEXT NULL,
    processed_at TIMESTAMPTZ NULL,
    dead_lettered_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT outbox_events_topic_check CHECK (
        topic IN (
            'invitation_created',
            'invitation_updated',
            'invitation_accepted',
            'invitation_rejected',
            'invitation_cancelled',
            'thread_created',
            'thread_reply_created',
            'mention_created'
        )
    ),
    CONSTRAINT outbox_events_aggregate_type_check CHECK (
        aggregate_type IN ('invitation', 'thread', 'thread_message')
    ),
    CONSTRAINT outbox_events_status_check CHECK (
        status IN ('pending', 'processing', 'processed', 'dead_letter')
    ),
    CONSTRAINT outbox_events_payload_object_check CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT outbox_events_attempt_count_check CHECK (attempt_count >= 0),
    CONSTRAINT outbox_events_max_attempts_check CHECK (max_attempts > 0),
    CONSTRAINT outbox_events_pending_state_check CHECK (
        status <> 'pending'
        OR (
            claimed_by IS NULL
            AND claimed_at IS NULL
            AND lease_expires_at IS NULL
            AND processed_at IS NULL
            AND dead_lettered_at IS NULL
        )
    ),
    CONSTRAINT outbox_events_processing_state_check CHECK (
        status <> 'processing'
        OR (
            claimed_by IS NOT NULL
            AND claimed_at IS NOT NULL
            AND lease_expires_at IS NOT NULL
            AND processed_at IS NULL
            AND dead_lettered_at IS NULL
        )
    ),
    CONSTRAINT outbox_events_processed_state_check CHECK (
        status <> 'processed'
        OR (
            claimed_by IS NOT NULL
            AND claimed_at IS NOT NULL
            AND lease_expires_at IS NULL
            AND processed_at IS NOT NULL
            AND dead_lettered_at IS NULL
        )
    ),
    CONSTRAINT outbox_events_dead_letter_state_check CHECK (
        status <> 'dead_letter'
        OR (
            claimed_by IS NOT NULL
            AND claimed_at IS NOT NULL
            AND lease_expires_at IS NULL
            AND processed_at IS NULL
            AND dead_lettered_at IS NOT NULL
        )
    )
);

CREATE UNIQUE INDEX outbox_events_idempotency_key_idx
    ON outbox_events (idempotency_key);

CREATE INDEX outbox_events_pending_claim_idx
    ON outbox_events (status, available_at ASC, created_at ASC, id ASC);

CREATE INDEX outbox_events_processing_lease_idx
    ON outbox_events (status, lease_expires_at ASC);

CREATE INDEX outbox_events_aggregate_audit_idx
    ON outbox_events (aggregate_type, aggregate_id, created_at ASC);
