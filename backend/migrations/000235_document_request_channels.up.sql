-- Belge talebi kanalları (Oğuz'un geri bildirimi): ofis talebi açarken
-- mükellefe hangi kanallardan haber verileceğini seçer (uygulama bildirimi,
-- SMS, e-posta), son tarihi ve isteğe bağlı hatırlatmayı (son tarihten N gün
-- önce) belirler.
--
-- channels boşsa eski davranış sürer (üyelere uygulama + e-posta bildirimi,
-- geri çekilmeli genel hatırlatma). channel_remind_at seçili kanallardan
-- gidecek tek hatırlatmanın zamanıdır; gidince channel_reminded_at yazılır.
ALTER TABLE document_tasks
    ADD COLUMN channels            TEXT[]      NOT NULL DEFAULT '{}',
    ADD COLUMN remind_days_before  SMALLINT    NULL,
    ADD COLUMN channel_remind_at   TIMESTAMPTZ NULL,
    ADD COLUMN channel_reminded_at TIMESTAMPTZ NULL;
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_channels
    CHECK (channels <@ ARRAY['push', 'sms', 'email']::text[]);
ALTER TABLE document_tasks ADD CONSTRAINT chk_document_tasks_remind_days
    CHECK (remind_days_before IS NULL OR remind_days_before BETWEEN 0 AND 30);
CREATE INDEX idx_document_tasks_channel_remind ON document_tasks (channel_remind_at)
    WHERE status = 'open' AND channel_remind_at IS NOT NULL;

-- Uygulama bildirimi kaydı: iletişim geçmişinde SMS ve e-postanın yanında
-- "push" (uygulama bildirimi) kanalı. Bildirim mükellefin uygulama kullanıcılarına (mobil
-- push dahil) gider; recipients kaç kişiye gittiğidir. Kimse yoksa failed.
CREATE TABLE push_messages (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id    BIGINT       NULL,
    title            TEXT         NOT NULL,
    body             TEXT         NOT NULL,
    recipients       INTEGER      NOT NULL DEFAULT 0,
    status           TEXT         NOT NULL,
    error_code       TEXT         NOT NULL DEFAULT '',
    error_message    TEXT         NOT NULL DEFAULT '',
    source_rule      TEXT         NOT NULL DEFAULT '',
    source_ref       TEXT         NOT NULL DEFAULT '',
    sent_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    sent_at          TIMESTAMPTZ  NULL,
    CONSTRAINT uq_push_messages_uuid UNIQUE (uuid),
    CONSTRAINT fk_push_messages_engagement FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_push_messages_status CHECK (status IN ('sent', 'failed')),
    CONSTRAINT chk_push_messages_recipients CHECK (recipients >= 0)
);
CREATE INDEX idx_push_messages_engagement ON push_messages (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_push_messages_source ON push_messages (organization_id, source_ref) WHERE source_ref <> '';
CREATE INDEX idx_push_messages_sent_by ON push_messages (sent_by);
SELECT rls_apply_org('push_messages');

-- Belge talebi kaynaklı SMS / e-posta kayıtlarının kanal durumunu hızlı
-- bulmak için.
CREATE INDEX idx_sms_messages_source ON sms_messages (organization_id, source_ref) WHERE source_ref <> '';
CREATE INDEX idx_mail_messages_source ON mail_messages (organization_id, source_ref) WHERE source_ref <> '';
