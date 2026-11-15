-- Tenant-core faz 6c (issue #4): mükellef kullanıcıları (B2B2C, G4).
--
-- taxpayer_members: ofis üyesi olmayan, kendi şirketine (mükellefe) bağlı
-- kullanıcılar. Bir kullanıcı birden çok mükellefte, aynı anda bir ofiste
-- de üye olabilir. Satırın varlığı üyeliğin aktif olduğu anlamına gelir.
-- Rol: owner (sahip) | employee (çalışan).
CREATE TABLE taxpayer_members (
    id          BIGSERIAL   PRIMARY KEY,
    taxpayer_id BIGINT      NOT NULL REFERENCES taxpayers (id) ON DELETE CASCADE,
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role        VARCHAR(16) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayer_members UNIQUE (taxpayer_id, user_id),
    CONSTRAINT chk_taxpayer_members_role CHECK (role IN ('owner', 'employee'))
);

CREATE INDEX idx_taxpayer_members_user ON taxpayer_members (user_id);

CREATE TRIGGER trg_taxpayer_members_set_updated_at
    BEFORE UPDATE ON taxpayer_members
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- taxpayer_invitations: mükellef kullanıcısı yalnızca ofis davetiyle gelir
-- (D15). Davet bir ofisin aktif engagement'ından gönderilir; ofisin iş
-- ürünüdür (organization_id + engagement_id). Yalnızca token'ın özeti saklanır.
CREATE TABLE taxpayer_invitations (
    id              BIGSERIAL   PRIMARY KEY,
    uuid            UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT      NOT NULL,
    engagement_id   BIGINT      NOT NULL,
    email           CITEXT      NOT NULL,
    role            VARCHAR(16) NOT NULL,
    token_hash      TEXT        NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    accepted_at     TIMESTAMPTZ NULL,
    revoked_at      TIMESTAMPTZ NULL,
    invited_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_taxpayer_invitations_uuid UNIQUE (uuid),
    CONSTRAINT uq_taxpayer_invitations_token_hash UNIQUE (token_hash),
    CONSTRAINT chk_taxpayer_invitations_role CHECK (role IN ('owner', 'employee')),
    CONSTRAINT fk_taxpayer_invitations_engagement
        FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_taxpayer_invitations_engagement
    ON taxpayer_invitations (organization_id, engagement_id, created_at DESC);
CREATE INDEX idx_taxpayer_invitations_pending
    ON taxpayer_invitations (engagement_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- Davet e-postası. Değişkenler: taxpayer_name, org_name, inviter_name,
-- role_name, link, expires_days.
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('taxpayer.invitation', 'email', 'tr', '{{org_name}} sizi {{taxpayer_name}} hesabına davet etti',
     'Merhaba, {{inviter_name}} ({{org_name}}) sizi {{taxpayer_name}} şirketinin mükellef paneline {{role_name}} olarak davet etti. Daveti kabul etmek için bağlantıyı açın: {{link}} (Bağlantı {{expires_days}} gün geçerlidir.)', TRUE),
    ('taxpayer.invitation', 'email', 'en', '{{org_name}} invited you to {{taxpayer_name}}',
     'Hello, {{inviter_name}} ({{org_name}}) invited you to the taxpayer panel of {{taxpayer_name}} as {{role_name}}. Open this link to accept: {{link}} (The link is valid for {{expires_days}} days.)', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
