ALTER TABLE workspace_invitations
    ADD COLUMN IF NOT EXISTS status TEXT,
    ADD COLUMN IF NOT EXISTS version BIGINT,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS responded_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS responded_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancelled_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

UPDATE workspace_invitations
SET status = CASE
        WHEN accepted_at IS NULL THEN 'pending'
        ELSE 'accepted'
    END,
    version = 1,
    updated_at = CASE
        WHEN accepted_at IS NULL THEN created_at
        ELSE accepted_at
    END,
    responded_by = NULL,
    responded_at = CASE
        WHEN accepted_at IS NULL THEN NULL
        ELSE accepted_at
    END,
    cancelled_by = NULL,
    cancelled_at = NULL
WHERE status IS NULL
   OR version IS NULL
   OR updated_at IS NULL;

ALTER TABLE workspace_invitations
    ALTER COLUMN status SET NOT NULL,
    ALTER COLUMN version SET NOT NULL,
    ALTER COLUMN updated_at SET NOT NULL;

ALTER TABLE workspace_invitations
    ADD CONSTRAINT workspace_invitations_status_check
        CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled')),
    ADD CONSTRAINT workspace_invitations_version_positive_check
        CHECK (version >= 1),
    ADD CONSTRAINT workspace_invitations_status_accepted_at_check
        CHECK (
            (status = 'accepted' AND accepted_at IS NOT NULL)
            OR (status <> 'accepted' AND accepted_at IS NULL)
        );

DROP INDEX IF EXISTS workspace_invitations_active_email_idx;

CREATE UNIQUE INDEX IF NOT EXISTS workspace_invitations_pending_email_idx
    ON workspace_invitations (workspace_id, email)
    WHERE status = 'pending';
