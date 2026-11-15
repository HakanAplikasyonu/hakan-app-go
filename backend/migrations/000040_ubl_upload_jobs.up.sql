-- UBL / ZIP yükleme işleri (issue #11). Ofisin işi: organization_id + engagement_id.
-- Dosya nesne deposunda (file_key); rapor eklenen / mükerrer / hatalı sayılarını ve
-- dosya bazında sonucu tutar.
CREATE TABLE ubl_upload_jobs (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL,
    engagement_id    BIGINT       NOT NULL,
    taxpayer_id      BIGINT       NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    actor_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    file_name        TEXT         NOT NULL,
    file_key         TEXT         NOT NULL,
    sha256           CHAR(64)     NOT NULL,
    size_bytes       BIGINT       NOT NULL,
    status           TEXT         NOT NULL DEFAULT 'queued',
    report           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error            TEXT         NULL,
    started_at       TIMESTAMPTZ  NULL,
    finished_at      TIMESTAMPTZ  NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ubl_upload_jobs_uuid UNIQUE (uuid),
    CONSTRAINT fk_ubl_upload_jobs_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_ubl_upload_jobs_status CHECK (status IN ('queued', 'processing', 'done', 'failed')),
    CONSTRAINT chk_ubl_upload_jobs_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_ubl_upload_jobs_list ON ubl_upload_jobs (organization_id, engagement_id, created_at DESC);

CREATE TRIGGER trg_ubl_upload_jobs_set_updated_at
    BEFORE UPDATE ON ubl_upload_jobs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Fatura izinleri: şablon rollerin hepsi okur ve yükler (müşavir ve personel
-- günlük işi). Erişim yine engagement ataması ile sınırlıdır.
INSERT INTO permissions (name, slug) VALUES
    ('Read tenant invoices', 'tenant.invoices.read'),
    ('Write tenant invoices', 'tenant.invoices.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN ('tenant.invoices.read', 'tenant.invoices.write')
WHERE r.slug IN ('owner', 'admin', 'member') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
