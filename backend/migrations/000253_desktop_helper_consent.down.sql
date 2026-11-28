ALTER TABLE desktop_agents DROP COLUMN IF EXISTS helper_seen_at;

DROP INDEX IF EXISTS idx_desktop_helper_settings_consent_agent;

ALTER TABLE desktop_helper_settings
    DROP COLUMN IF EXISTS luca_consent_text_version,
    DROP COLUMN IF EXISTS luca_consent_agent_id,
    DROP COLUMN IF EXISTS luca_consent_by,
    DROP COLUMN IF EXISTS luca_consent_at;
