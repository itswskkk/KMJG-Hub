-- 0014_project_lifecycle.sql
-- Project soft-deletion (30-day recovery) and ownership-transfer audit.
-- Implements docs/PRD.md § Project Management (Ownership Transfer,
-- Project Deletion, Project Recovery).

ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
  -- The Project Owner at the time of deletion — the only user (besides the
  -- Server Administrator) permitted to restore it, per docs/PRD.md "the
  -- user who was the Project Owner at the time of deletion may restore the
  -- Project". Recorded separately from project_members.role because
  -- membership can change (or the row could theoretically be affected)
  -- during the retention window.
  ADD COLUMN IF NOT EXISTS deleted_by_owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- Normal queries must filter deleted_at IS NULL; a partial index keeps that
-- filter cheap without penalizing the (presumably rare) deleted-projects lookup.
CREATE INDEX IF NOT EXISTS idx_projects_active ON projects(id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_projects_deleted ON projects(deleted_by_owner_user_id, deleted_at) WHERE deleted_at IS NOT NULL;
