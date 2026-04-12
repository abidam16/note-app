DROP INDEX IF EXISTS workspace_invitations_pending_email_idx;

ALTER TABLE workspace_invitations
    DROP CONSTRAINT IF EXISTS workspace_invitations_status_accepted_at_check,
    DROP CONSTRAINT IF EXISTS workspace_invitations_version_positive_check,
    DROP CONSTRAINT IF EXISTS workspace_invitations_status_check;

ALTER TABLE workspace_invitations
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS cancelled_by,
    DROP COLUMN IF EXISTS responded_at,
    DROP COLUMN IF EXISTS responded_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS status;

CREATE UNIQUE INDEX IF NOT EXISTS workspace_invitations_active_email_idx
    ON workspace_invitations (workspace_id, email)
    WHERE accepted_at IS NULL;
