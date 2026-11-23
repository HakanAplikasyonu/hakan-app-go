DROP INDEX IF EXISTS idx_mail_messages_source;
DROP INDEX IF EXISTS idx_sms_messages_source;
DROP TABLE IF EXISTS push_messages;
DROP INDEX IF EXISTS idx_document_tasks_channel_remind;
ALTER TABLE document_tasks DROP CONSTRAINT IF EXISTS chk_document_tasks_remind_days;
ALTER TABLE document_tasks DROP CONSTRAINT IF EXISTS chk_document_tasks_channels;
ALTER TABLE document_tasks
    DROP COLUMN IF EXISTS channel_reminded_at,
    DROP COLUMN IF EXISTS channel_remind_at,
    DROP COLUMN IF EXISTS remind_days_before,
    DROP COLUMN IF EXISTS channels;
