-- KVKK veri sahibi talepleri (issue #193, KVKK md. 11): mükellefin ya da
-- mükellef kullanıcısının kişisel verilerinin dışa aktarımı ve silme /
-- anonimleştirme. Talebi ofis çalışanı açar, ofis yöneticisi onaylar;
-- onayla iş yürütülür. Kayıt silinmez: ne yapıldığı summary'de kalır.
-- Muhasebe kayıtları (fatura, banka, gider, yevmiye) talepten etkilenmez.
CREATE TABLE privacy_requests (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    kind            TEXT         NOT NULL,
    -- Kimin verisi: mükellefin kendisi ya da onun portal kullanıcısı.
    subject_type    TEXT         NOT NULL,
    subject_user_id BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    status          TEXT         NOT NULL DEFAULT 'pending',
    reason          TEXT         NOT NULL DEFAULT '',
    decision_note   TEXT         NOT NULL DEFAULT '',
    summary         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    requested_by    BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    decided_at      TIMESTAMPTZ  NULL,
    completed_at    TIMESTAMPTZ  NULL,
    -- Dışa aktarım bu zamana kadar indirilebilir.
    expires_at      TIMESTAMPTZ  NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_privacy_requests_uuid UNIQUE (uuid),
    CONSTRAINT chk_privacy_requests_kind CHECK (kind IN ('export', 'erasure')),
    CONSTRAINT chk_privacy_requests_subject CHECK (
        (subject_type = 'taxpayer') OR (subject_type = 'user' AND (subject_user_id IS NOT NULL OR status <> 'pending'))),
    CONSTRAINT chk_privacy_requests_subject_type CHECK (subject_type IN ('taxpayer', 'user')),
    CONSTRAINT chk_privacy_requests_status CHECK (status IN ('pending', 'rejected', 'completed', 'failed')),
    CONSTRAINT fk_privacy_requests_engagement
        FOREIGN KEY (organization_id, engagement_id)
        REFERENCES engagements (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_privacy_requests_engagement ON privacy_requests (organization_id, engagement_id, created_at DESC);

CREATE TRIGGER trg_privacy_requests_set_updated_at
    BEFORE UPDATE ON privacy_requests
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

SELECT rls_apply_engagement('privacy_requests');

-- request: talep açma ve görme (owner, admin, member). approve: onay/ret ve
-- dışa aktarımı indirme (owner, admin).
INSERT INTO permissions (name, slug) VALUES
    ('Create and view KVKK data subject requests', 'tenant.privacy.request'),
    ('Approve KVKK data subject requests and download exports', 'tenant.privacy.approve')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.privacy.request', 'tenant.privacy.approve')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug = 'tenant.privacy.request'
WHERE r.slug = 'member' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
