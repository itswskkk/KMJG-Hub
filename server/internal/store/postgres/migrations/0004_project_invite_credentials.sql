-- Shareable Project invite links/codes. Only a SHA-256 hash of the
-- generated secret is persisted, so a database read cannot redeem it.
CREATE TABLE IF NOT EXISTS project_invite_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    creator_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    max_uses INTEGER,
    uses INTEGER NOT NULL DEFAULT 0 CHECK (uses >= 0),
    revoked_at TIMESTAMPTZ,
    CHECK (max_uses IS NULL OR max_uses > 0)
);

CREATE INDEX IF NOT EXISTS project_invite_credentials_project_active_idx
    ON project_invite_credentials (project_id, created_at DESC)
    WHERE revoked_at IS NULL;
