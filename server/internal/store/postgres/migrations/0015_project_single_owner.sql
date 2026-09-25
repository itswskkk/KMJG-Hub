-- 0015_project_single_owner.sql
-- docs/PRD.md "Ownership Transfer": "The Project continues to have exactly
-- one Owner." Application code already enforces this (ownership only moves
-- via ProjectRepository.TransferOwnership, which demotes the previous Owner
-- before promoting the new one); this partial unique index makes a second
-- Owner row impossible at the database level too.
CREATE UNIQUE INDEX IF NOT EXISTS project_members_single_owner_idx
  ON project_members (project_id) WHERE role = 'owner';
