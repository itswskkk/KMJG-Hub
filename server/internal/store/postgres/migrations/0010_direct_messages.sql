-- 0010_direct_messages.sql
-- Direct Messages between users who are friends or share an active project
-- Implements docs/PRD.md § Friends and Direct Messages, Project Member Messaging

CREATE TABLE IF NOT EXISTS direct_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  deleted_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  CHECK (sender_id != recipient_id),
  CHECK (char_length(body) <= 4000),
  CHECK ((deleted_at IS NULL) = (deleted_by_user_id IS NULL))
);

-- Keyset pagination: (created_at, id) per conversation pair
CREATE INDEX IF NOT EXISTS idx_dm_conversation
  ON direct_messages (LEAST(sender_id, recipient_id), GREATEST(sender_id, recipient_id), created_at DESC, id DESC)
  WHERE deleted_at IS NULL;

-- For listing active conversations per user (last message)
CREATE INDEX IF NOT EXISTS idx_dm_sender_created ON direct_messages(sender_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dm_recipient_created ON direct_messages(recipient_id, created_at DESC);

-- For retention sweep
CREATE INDEX IF NOT EXISTS idx_dm_deleted_at ON direct_messages(deleted_at) WHERE deleted_at IS NOT NULL;
