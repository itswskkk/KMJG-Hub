-- 0016_chat_system_messages.sql
-- Server-generated Project Chat entries (docs/PRD.md "Git Push
-- Notifications": "Git push activity may be posted as system activity
-- inside the Project Chat"). Such entries have no user author, and are not
-- ordinary user-authored messages for deletion permissions (docs/PRD.md
-- "Configured Git activity and Server-generated system activity are not
-- treated as ordinary user-authored messages").
--
-- kind:
--   'user' — a normal member message (author_user_id required)
--   'git'  — configured Git activity posted by the Server (no author)

ALTER TABLE project_chat_messages ALTER COLUMN author_user_id DROP NOT NULL;

ALTER TABLE project_chat_messages
  ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'user';

ALTER TABLE project_chat_messages DROP CONSTRAINT IF EXISTS project_chat_messages_kind_check;
ALTER TABLE project_chat_messages ADD CONSTRAINT project_chat_messages_kind_check
  CHECK (
    (kind = 'user' AND author_user_id IS NOT NULL)
    OR (kind = 'git' AND author_user_id IS NULL)
  );

-- The Files section lists a Project's attachments newest first.
CREATE INDEX IF NOT EXISTS project_chat_attachments_project_created_idx
  ON project_chat_attachments (project_id, created_at DESC, id DESC);
