-- 0012_github.sql
-- GitHub OAuth identity + per-Project repository connection.
-- Implements docs/PRD.md § GitHub Authentication, Project Git Integration.

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS github_user_id BIGINT UNIQUE,
  ADD COLUMN IF NOT EXISTS github_login VARCHAR(255),
  -- Tokens are encrypted at rest by the application layer before storage;
  -- this column holds ciphertext, never a plaintext token.
  ADD COLUMN IF NOT EXISTS github_access_token_encrypted BYTEA,
  ADD COLUMN IF NOT EXISTS github_connected_at TIMESTAMPTZ;

-- KMJG Hub v1: one Project may be associated with a maximum of one Git
-- repository (docs/PRD.md "Core Product Model"). The model is kept
-- provider-neutral in naming (`provider`, not github-specific columns on
-- `projects` itself) so future non-GitHub providers do not require a schema
-- rename, per docs/PRD.md "The repository integration should not
-- permanently depend on GitHub-specific repository URLs".
CREATE TABLE IF NOT EXISTS project_repositories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
  provider VARCHAR(32) NOT NULL DEFAULT 'github' CHECK (provider IN ('github')),
  external_repo_id BIGINT NOT NULL,
  owner_login VARCHAR(255) NOT NULL,
  repo_name VARCHAR(255) NOT NULL,
  html_url VARCHAR(512) NOT NULL,
  default_branch VARCHAR(255) NOT NULL DEFAULT 'main',
  -- Nullable: the connecting user's account may later be deleted without
  -- disconnecting the Project's repository.
  connected_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  connected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Git push notification configuration (docs/PRD.md "Git Push Notifications")
  post_pushes_to_chat BOOLEAN NOT NULL DEFAULT false,
  notify_all_members BOOLEAN NOT NULL DEFAULT true,
  notify_all_branches BOOLEAN NOT NULL DEFAULT true
);

-- Incoming push webhooks are routed by the provider's repository ID.
CREATE INDEX IF NOT EXISTS idx_project_repositories_external
  ON project_repositories(provider, external_repo_id);

-- Deployment-level OAuth App credentials and the webhook secret are read
-- from the Server environment (KMJG_GITHUB_* variables, see
-- internal/config), never stored in this database, per docs/PRD.md "Open
-- Source and Privacy".
