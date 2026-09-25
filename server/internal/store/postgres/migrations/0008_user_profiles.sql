-- 0008_user_profiles.sql
-- User profiles with field-level privacy controls
-- Extends docs/PRD.md § User Profiles and docs/ARCHITECTURE.md

-- Add profile fields to users table
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS display_name VARCHAR(255),
  ADD COLUMN IF NOT EXISTS avatar VARCHAR(2048),
  ADD COLUMN IF NOT EXISTS bio TEXT;

-- Profile field privacy settings
-- Each user controls visibility per field: everyone, friends, project_members, friends_and_project_members, nobody
CREATE TABLE IF NOT EXISTS profile_privacy (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  field VARCHAR(64) NOT NULL, -- display_name, avatar, bio, current_project, current_task, current_branch, repositories
  audience VARCHAR(64) NOT NULL, -- everyone, friends, project_members, friends_and_project_members, nobody
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (field IN ('display_name', 'avatar', 'bio', 'current_project', 'current_task', 'current_branch', 'repositories')),
  CHECK (audience IN ('everyone', 'friends', 'project_members', 'friends_and_project_members', 'nobody')),
  UNIQUE (user_id, field)
);

CREATE INDEX IF NOT EXISTS idx_profile_privacy_user_id ON profile_privacy(user_id);
