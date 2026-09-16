-- Projects and Project membership.
-- See docs/PRD.md "Core Product Model" and "Roles and Permissions",
-- docs/ARCHITECTURE.md "Resource-Level Authorization".
--
-- Git repository connection is intentionally not modeled yet: no feature
-- writes it in this checkpoint (see internal/project package docs), so
-- there is nothing for those columns to store. They belong to the Git
-- Provider Integration checkpoint instead of being guessed at here.

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX IF NOT EXISTS project_members_user_id_idx ON project_members (user_id);
