-- 0013_file_transfer.sql
-- Direct File Transfer: request -> accept/decline -> upload -> download.
-- Implements docs/PRD.md § File Sharing and Transfer, Direct File Transfer.
--
-- Unlike Project Chat attachments, the file is NEVER uploaded to server
-- storage until the recipient explicitly accepts (docs/PRD.md: "The file
-- must not be uploaded to server storage before the recipient accepts").

CREATE TABLE IF NOT EXISTS file_transfers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  file_name VARCHAR(512) NOT NULL,
  declared_file_size BIGINT NOT NULL CHECK (declared_file_size > 0),
  status VARCHAR(16) NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'accepted', 'declined', 'uploaded', 'cancelled', 'expired')),
  storage_id VARCHAR(255), -- set only after successful upload (post-accept)
  actual_file_size BIGINT, -- set only after successful upload; must match declared size
  content_type VARCHAR(255),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  responded_at TIMESTAMPTZ,
  uploaded_at TIMESTAMPTZ,
  CHECK (sender_id != recipient_id),
  CHECK ((status = 'uploaded') = (storage_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_file_transfers_recipient ON file_transfers(recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_transfers_sender ON file_transfers(sender_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_transfers_status ON file_transfers(status) WHERE status IN ('pending', 'accepted');
