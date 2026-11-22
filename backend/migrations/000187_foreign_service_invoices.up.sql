-- Yabancı hizmet faturaları (#780, KDV2 A1): Google Ads, AWS, yurt dışı
-- yazılım lisansı gibi yurt dışından alınan hizmetlerin faturaları. Beyan
-- Luca'da verilir; burada faturalar toplanır, sorumlu sıfatıyla KDV (KDV2)
-- ve gerekiyorsa stopaj hesaplanır, ofis onaylar. Hesaplanan tutarlar
-- saklanmaz, Go'da (modules/foreigninvoices/compute.go) hesaplanır.
--
-- Sonraki işler bu tabloyu kullanır: A2 mükellef yüklemesi ve AI ön
-- doldurma (source_kind / source_uuid kaynak belgeye bağlanır), B fiş
-- taslağı ve Luca aktarımı, C KDV2 ve muhtasar beyanını bu listeden besleme.
CREATE TABLE foreign_service_invoices (
    id               BIGSERIAL      PRIMARY KEY,
    uuid             UUID           NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT         NOT NULL,
    engagement_id    BIGINT         NOT NULL,
    period           CHAR(7)        NOT NULL,
    vendor           TEXT           NOT NULL,
    -- ISO 3166-1 alpha-2 (IE, US, DE...).
    country          CHAR(2)        NOT NULL,
    invoice_no       TEXT           NOT NULL DEFAULT '',
    invoice_date     DATE           NULL,
    -- Faturanın tutarı (brütleştirmede ödenen net tutar), fatura dövizinde.
    amount           NUMERIC(18,2)  NOT NULL,
    currency         CHAR(3)        NOT NULL DEFAULT 'USD',
    -- 1 döviz = exchange_rate TL; NULL: kur henüz girilmedi (TL tutarlar boş).
    exchange_rate    NUMERIC(18,6)  NULL,
    -- yazilim_lisans, reklam, bulut, danismanlik, diger (Go'da sabit liste).
    service_kind     TEXT           NOT NULL,
    -- kdv2: yalnız sorumlu sıfatıyla KDV; kdv2_stopaj: KDV2 ve stopaj.
    treatment        TEXT           NOT NULL,
    stopaj_rate      NUMERIC(5,2)   NOT NULL DEFAULT 0,
    gross_up         BOOLEAN        NOT NULL DEFAULT FALSE,
    vat_rate         NUMERIC(5,2)   NOT NULL DEFAULT 20,
    status           TEXT           NOT NULL DEFAULT 'draft',
    -- Kaynak belge (A2: mükellef yüklemesi, WhatsApp, e-posta); şimdilik boş.
    source_kind      TEXT           NULL,
    source_uuid      UUID           NULL,
    note             TEXT           NULL,
    created_by       BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    confirmed_by     BIGINT         NULL REFERENCES users (id) ON DELETE SET NULL,
    confirmed_at     TIMESTAMPTZ    NULL,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_foreign_service_invoices_uuid UNIQUE (uuid),
    CONSTRAINT fk_foreign_service_invoices_engagement
        FOREIGN KEY (organization_id, engagement_id) REFERENCES engagements (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_foreign_service_invoices_period CHECK (period ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_foreign_service_invoices_country CHECK (country ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_foreign_service_invoices_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_foreign_service_invoices_amount CHECK (amount > 0),
    CONSTRAINT chk_foreign_service_invoices_rate CHECK (exchange_rate IS NULL OR exchange_rate > 0),
    CONSTRAINT chk_foreign_service_invoices_treatment CHECK (treatment IN ('kdv2', 'kdv2_stopaj')),
    CONSTRAINT chk_foreign_service_invoices_stopaj CHECK (stopaj_rate >= 0 AND stopaj_rate < 100),
    CONSTRAINT chk_foreign_service_invoices_vat CHECK (vat_rate >= 0 AND vat_rate <= 100),
    CONSTRAINT chk_foreign_service_invoices_status CHECK (status IN ('draft', 'confirmed'))
);

CREATE INDEX idx_foreign_service_invoices_engagement
    ON foreign_service_invoices (organization_id, engagement_id, period, id);
CREATE INDEX idx_foreign_service_invoices_created_by ON foreign_service_invoices (created_by);
CREATE INDEX idx_foreign_service_invoices_confirmed_by ON foreign_service_invoices (confirmed_by);

SELECT rls_apply_engagement('foreign_service_invoices'::regclass);

-- Ofis bazında işlem türü varsayılanları. Satırı olmayan tür Go'daki hazır
-- varsayılanı kullanır.
CREATE TABLE foreign_service_kind_settings (
    organization_id  BIGINT        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    service_kind     TEXT          NOT NULL,
    treatment        TEXT          NOT NULL,
    stopaj_rate      NUMERIC(5,2)  NOT NULL DEFAULT 0,
    gross_up         BOOLEAN       NOT NULL DEFAULT FALSE,
    vat_rate         NUMERIC(5,2)  NOT NULL DEFAULT 20,
    enabled          BOOLEAN       NOT NULL DEFAULT TRUE,
    updated_by       BIGINT        NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, service_kind),
    CONSTRAINT chk_foreign_service_kind_settings_treatment CHECK (treatment IN ('kdv2', 'kdv2_stopaj')),
    CONSTRAINT chk_foreign_service_kind_settings_stopaj CHECK (stopaj_rate >= 0 AND stopaj_rate < 100),
    CONSTRAINT chk_foreign_service_kind_settings_vat CHECK (vat_rate >= 0 AND vat_rate <= 100)
);

CREATE INDEX idx_foreign_service_kind_settings_updated_by ON foreign_service_kind_settings (updated_by);

SELECT rls_apply_org('foreign_service_kind_settings');

-- Ofis ayarı: "Yabancı faturalar / KDV2" bölümü açık/kapalı (varsayılan
-- açık). Satırı olmayan ofis açık sayılır.
CREATE TABLE foreign_invoice_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled          BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_foreign_invoice_settings_updated_by ON foreign_invoice_settings (updated_by);

SELECT rls_apply_org('foreign_invoice_settings');
