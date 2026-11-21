-- İletişim merkezi (#780 A bölümü, ilk parça): ofis bazlı NetGSM hesabı ve
-- giden SMS / e-posta kayıtları (gönderim kanıtı).
--
-- NetGSM API şifresi bu tabloda tutulmaz: kimlik kasasına (secrets vault)
-- yazılır, burada yalnız kasadaki kaydın referansı durur. Ekran şifreyi hiç
-- almaz, yalnız "şifre kayıtlı" bilgisini görür.
CREATE TABLE office_sms_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled          BOOLEAN      NOT NULL DEFAULT FALSE,
    -- Deneme modu: kayıt yazılır, NetGSM'e istek gitmez. Yeni ofiste açık.
    test_mode        BOOLEAN      NOT NULL DEFAULT TRUE,
    usercode         TEXT         NOT NULL DEFAULT '',
    password_ref     TEXT         NULL,
    msgheader        TEXT         NOT NULL DEFAULT '',
    encoding         TEXT         NOT NULL DEFAULT 'TR',
    appname          TEXT         NOT NULL DEFAULT '',
    -- Son bağlantı testi: zaman, sonuç, NetGSM kodu ve dönen gönderici adları.
    last_test_at     TIMESTAMPTZ  NULL,
    last_test_ok     BOOLEAN      NULL,
    last_test_code   TEXT         NOT NULL DEFAULT '',
    last_test_headers TEXT[]      NOT NULL DEFAULT '{}',
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_office_sms_usercode CHECK (usercode ~ '^[0-9]{0,20}$'),
    CONSTRAINT chk_office_sms_msgheader CHECK (msgheader = '' OR char_length(msgheader) BETWEEN 3 AND 11),
    CONSTRAINT chk_office_sms_encoding CHECK (encoding IN ('TR', 'UTF-8', 'UNICODE')),
    CONSTRAINT chk_office_sms_appname CHECK (char_length(appname) <= 64)
);
CREATE INDEX idx_office_sms_settings_updated_by ON office_sms_settings (updated_by);
SELECT rls_apply_org('office_sms_settings');

-- Giden SMS kaydı. Durum: pending (bekliyor), sent (gönderildi), failed
-- (başarısız), uncertain (cevap beklerken bağlantı koptu, sonuç belirsiz),
-- dry_run (deneme modu, gönderilmedi).
CREATE TABLE sms_messages (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id    BIGINT       NULL,
    phone            TEXT         NOT NULL,
    body             TEXT         NOT NULL,
    msgheader        TEXT         NOT NULL DEFAULT '',
    status           TEXT         NOT NULL DEFAULT 'pending',
    provider_job_id  TEXT         NOT NULL DEFAULT '',
    error_code       TEXT         NOT NULL DEFAULT '',
    error_message    TEXT         NOT NULL DEFAULT '',
    -- Kaynak: otomasyon kuralı kodu ya da kaynak kayıt (ör. tahakkuk uuid'i).
    source_rule      TEXT         NOT NULL DEFAULT '',
    source_ref       TEXT         NOT NULL DEFAULT '',
    sent_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    bulk_id          UUID         NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    sent_at          TIMESTAMPTZ  NULL,
    delivered_at     TIMESTAMPTZ  NULL,
    CONSTRAINT uq_sms_messages_uuid UNIQUE (uuid),
    CONSTRAINT fk_sms_messages_engagement FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_sms_messages_status CHECK (status IN ('pending', 'sent', 'failed', 'uncertain', 'dry_run'))
);
CREATE INDEX idx_sms_messages_org_created ON sms_messages (organization_id, created_at DESC);
CREATE INDEX idx_sms_messages_engagement ON sms_messages (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_sms_messages_sent_by ON sms_messages (sent_by);
CREATE INDEX idx_sms_messages_bulk ON sms_messages (organization_id, bulk_id) WHERE bulk_id IS NOT NULL;
SELECT rls_apply_org('sms_messages');

-- Giden e-posta kaydı: SMS ile aynı alanlar, ek olarak açılma (izleme
-- pikseli) ve indirme (ek bağlantısı) zamanları. track_token pikselin ve
-- bağlantının tahmin edilemez anahtarıdır.
CREATE TABLE mail_messages (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    engagement_id    BIGINT       NULL,
    to_email         CITEXT       NOT NULL,
    subject          TEXT         NOT NULL,
    body             TEXT         NOT NULL,
    status           TEXT         NOT NULL DEFAULT 'pending',
    error_message    TEXT         NOT NULL DEFAULT '',
    source_rule      TEXT         NOT NULL DEFAULT '',
    source_ref       TEXT         NOT NULL DEFAULT '',
    sent_by          BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    bulk_id          UUID         NULL,
    track_token      UUID         NOT NULL DEFAULT gen_random_uuid(),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    sent_at          TIMESTAMPTZ  NULL,
    opened_at        TIMESTAMPTZ  NULL,
    downloaded_at    TIMESTAMPTZ  NULL,
    CONSTRAINT uq_mail_messages_uuid UNIQUE (uuid),
    CONSTRAINT uq_mail_messages_track UNIQUE (track_token),
    CONSTRAINT fk_mail_messages_engagement FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_mail_messages_status CHECK (status IN ('pending', 'sent', 'failed', 'dry_run')),
    CONSTRAINT chk_mail_messages_email CHECK (char_length(to_email) <= 254)
);
CREATE INDEX idx_mail_messages_org_created ON mail_messages (organization_id, created_at DESC);
CREATE INDEX idx_mail_messages_engagement ON mail_messages (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_mail_messages_sent_by ON mail_messages (sent_by);
CREATE INDEX idx_mail_messages_bulk ON mail_messages (organization_id, bulk_id) WHERE bulk_id IS NOT NULL;
SELECT rls_apply_org('mail_messages');
