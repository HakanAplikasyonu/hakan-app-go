-- Mükellef defterleri (#780): ofis her ay Luca'dan aldığı işletme defteri
-- dökümünü (Excel / CSV) ve e-defter dosyalarını (yevmiye, kebir, berat XML
-- ya da bunların zip'i) mükellefe ve döneme yükler. Dosya nesne deposunda
-- (ledgers/<org>/<engagement>/<sha256><uzantı>), satır burada; okunabildiyse
-- özet (summary) JSON'da. Okunamayan dosya da saklanır (readable = false).
--
-- Aynı iş (engagement) aynı dosyayı (sha256) bir kez tutar.
CREATE TABLE ledger_files (
    id              BIGSERIAL    PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    kind            TEXT         NOT NULL,
    period          CHAR(7)      NOT NULL,
    file_name       TEXT         NOT NULL,
    content_type    TEXT         NOT NULL,
    storage_key     TEXT         NOT NULL,
    sha256          TEXT         NOT NULL,
    size_bytes      BIGINT       NOT NULL,
    readable        BOOLEAN      NOT NULL DEFAULT FALSE,
    summary         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    uploaded_by     BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ledger_files_uuid UNIQUE (uuid),
    CONSTRAINT uq_ledger_files_sha UNIQUE (engagement_id, sha256),
    CONSTRAINT fk_ledger_files_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_ledger_files_kind CHECK (kind IN ('isletme_defteri', 'e_defter_yevmiye', 'e_defter_kebir', 'berat', 'other')),
    CONSTRAINT chk_ledger_files_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_ledger_files_size CHECK (size_bytes > 0),
    CONSTRAINT chk_ledger_files_sha CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_ledger_files_list ON ledger_files (organization_id, engagement_id, period);
CREATE INDEX idx_ledger_files_uploaded_by ON ledger_files (uploaded_by);

-- RLS (issue #6): engagement kapsamlı.
SELECT rls_apply_engagement('ledger_files'::regclass);

-- Defter dönemi (#780): mükellefin defteri aylık mı 3 aylık mı yüklenir.
-- Satır yoksa aylık. Eksik dönem uyarısı buna göre sayılır.
CREATE TABLE ledger_settings (
    id              BIGSERIAL    PRIMARY KEY,
    organization_id BIGINT       NOT NULL,
    engagement_id   BIGINT       NOT NULL,
    period_kind     TEXT         NOT NULL DEFAULT 'monthly',
    updated_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ledger_settings_engagement UNIQUE (engagement_id),
    CONSTRAINT fk_ledger_settings_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_ledger_settings_period_kind CHECK (period_kind IN ('monthly', 'quarterly'))
);

CREATE INDEX idx_ledger_settings_org_engagement ON ledger_settings (organization_id, engagement_id);
CREATE INDEX idx_ledger_settings_updated_by ON ledger_settings (updated_by);

SELECT rls_apply_engagement('ledger_settings'::regclass);
