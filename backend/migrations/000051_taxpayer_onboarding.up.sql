-- Mükellef ekleme akışı (issue #7, D1b, D15).
--
-- Ofis VKN/TCKN ve iletişim bilgisini girer; yanıt her durumda aynıdır
-- ("mükellefe onay gönderildi"). Talep oluşturulurken taxpayers tablosuna hiç
-- bakılmaz: talep satırı yalnızca ofisin girdiğini tutar, taxpayer_id ve
-- engagement_id mükellef onaylayınca dolar. Böylece VKN'nin sistemde olup
-- olmadığı ne yanıttan, ne hata kodundan, ne de süreden anlaşılır.
CREATE TABLE taxpayer_onboarding_requests (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind            VARCHAR(16)  NOT NULL,
    tax_id          VARCHAR(11)  NOT NULL,
    title           VARCHAR(255) NOT NULL,
    contact_email   CITEXT       NOT NULL,
    token_hash      TEXT         NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    requested_by    BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    taxpayer_id     BIGINT       NULL REFERENCES taxpayers (id) ON DELETE SET NULL,
    engagement_id   BIGINT       NULL REFERENCES engagements (id) ON DELETE SET NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    decided_at      TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayer_onboarding_requests_uuid UNIQUE (uuid),
    CONSTRAINT uq_taxpayer_onboarding_requests_token UNIQUE (token_hash),
    CONSTRAINT chk_taxpayer_onboarding_requests_kind CHECK (kind IN ('legal', 'individual')),
    CONSTRAINT chk_taxpayer_onboarding_requests_tax_id CHECK (tax_id ~ '^[0-9]{10,11}$'),
    CONSTRAINT chk_taxpayer_onboarding_requests_status CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled'))
);

CREATE INDEX idx_taxpayer_onboarding_requests_org
    ON taxpayer_onboarding_requests (organization_id, created_at DESC);
CREATE INDEX idx_taxpayer_onboarding_requests_tax_id_pending
    ON taxpayer_onboarding_requests (tax_id)
    WHERE status = 'pending';

SELECT rls_apply_org('taxpayer_onboarding_requests');

-- Mükellefe giden onay e-postası. Değişkenler: org_name, title, tax_id,
-- link, expires_days.
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('taxpayer.onboarding_consent', 'email', 'tr', '{{org_name}} sizi mükellef olarak eklemek istiyor',
     'Merhaba, {{org_name}} mali müşavirlik ofisi {{title}} ({{tax_id}}) için mükellef kaydı açmak istiyor. Onaylamak ya da reddetmek için bağlantıyı açın: {{link}} (Bağlantı {{expires_days}} gün geçerlidir.) Bu talebi siz beklemiyorsanız e-postayı yok sayabilirsiniz.', TRUE),
    ('taxpayer.onboarding_consent', 'email', 'en', '{{org_name}} wants to add you as a client',
     'Hello, the accounting office {{org_name}} wants to open a client record for {{title}} ({{tax_id}}). Open this link to approve or decline: {{link}} (The link is valid for {{expires_days}} days.) If you did not expect this, ignore this email.', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
