-- 0011_notifications.sql
-- Persistent, per-user notifications for important collaboration events.
-- Implements docs/PRD.md § Notifications.

CREATE TABLE IF NOT EXISTS notifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_type VARCHAR(64) NOT NULL,
  -- Free-form JSON payload carrying whatever context the event type needs
  -- (project_id, task_id, message_id, sender_id, etc.) — never trusted as
  -- an authorization grant; every notification's underlying resource is
  -- re-checked for current access when the Client acts on it.
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  read_at TIMESTAMPTZ,
  CHECK (event_type IN (
    'task_assigned',
    'task_comment',
    'direct_message',
    'file_transfer_request',
    'project_invitation',
    'role_changed',
    'git_push',
    'friend_request'
  ))
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_created ON notifications(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_id) WHERE read_at IS NULL;
