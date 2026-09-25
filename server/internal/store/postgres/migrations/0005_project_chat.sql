-- One persistent shared chat per Project. User-authored messages are
-- soft-deleted for the documented 30-day retention period; normal reads
-- exclude them immediately.
CREATE TABLE IF NOT EXISTS project_chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    author_user_id UUID NOT NULL REFERENCES users(id),
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    deleted_by_user_id UUID REFERENCES users(id),
    CHECK ((deleted_at IS NULL) = (deleted_by_user_id IS NULL))
);

CREATE INDEX IF NOT EXISTS project_chat_messages_active_timeline_idx
    ON project_chat_messages (project_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
