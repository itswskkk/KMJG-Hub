-- Project-scoped live development context. This is last-reported state only;
-- the realtime Hub remains authoritative for whether it may be presented.
CREATE TABLE IF NOT EXISTS project_member_work_contexts (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    working BOOLEAN NOT NULL DEFAULT FALSE,
    status_mode TEXT NOT NULL DEFAULT 'automatic'
        CHECK (status_mode IN ('automatic', 'manual')),
    current_branch TEXT CHECK (current_branch IS NULL OR char_length(current_branch) BETWEEN 1 AND 255),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id),
    FOREIGN KEY (project_id, user_id)
        REFERENCES project_members(project_id, user_id) ON DELETE CASCADE
);

-- File bytes live in the configured storage backend. PostgreSQL owns the
-- authorization and lifecycle metadata.
CREATE TABLE IF NOT EXISTS project_chat_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES project_chat_messages(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    storage_id TEXT NOT NULL UNIQUE,
    filename TEXT NOT NULL CHECK (char_length(filename) BETWEEN 1 AND 255),
    content_type TEXT NOT NULL CHECK (char_length(content_type) BETWEEN 1 AND 255),
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS project_chat_attachments_message_idx
    ON project_chat_attachments(message_id);
CREATE INDEX IF NOT EXISTS project_chat_attachments_project_idx
    ON project_chat_attachments(project_id);

-- Attachment-only messages are valid, while ordinary text sends remain
-- guarded by the service layer.
ALTER TABLE project_chat_messages DROP CONSTRAINT IF EXISTS project_chat_messages_body_check;
ALTER TABLE project_chat_messages ADD CONSTRAINT project_chat_messages_body_check
    CHECK (char_length(body) BETWEEN 0 AND 4000);
