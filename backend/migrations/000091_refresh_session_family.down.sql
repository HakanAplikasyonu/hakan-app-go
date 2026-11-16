DROP INDEX IF EXISTS idx_refresh_tokens_session_uuid;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS ended_at;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS session_uuid;
