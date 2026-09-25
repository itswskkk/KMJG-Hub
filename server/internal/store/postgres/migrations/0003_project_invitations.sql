-- Server-managed direct Project invitations. Invitation acceptance grants
-- KMJG Hub membership only; repository access remains a separate provider
-- operation (docs/ARCHITECTURE.md "Project Invitation Architecture").

CREATE TABLE IF NOT EXISTS project_direct_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    inviter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,
    declined_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    CHECK (inviter_user_id <> recipient_user_id),
    CHECK (num_nonnulls(accepted_at, declined_at, cancelled_at) <= 1)
);

CREATE INDEX IF NOT EXISTS project_direct_invitations_recipient_pending_idx
    ON project_direct_invitations (recipient_user_id, created_at DESC)
    WHERE accepted_at IS NULL AND declined_at IS NULL AND cancelled_at IS NULL;
