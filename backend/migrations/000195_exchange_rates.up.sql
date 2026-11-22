-- TCMB günlük döviz kurları (#780): worker her iş günü 15:30'dan sonra
-- https://www.tcmb.gov.tr/kurlar/today.xml'i, eksik geçmiş günler için
-- arşivi (kurlar/YYYYMM/DDMMYYYY.xml) çeker ve buraya yazar. Kiracısız,
-- herkese ortak tablo: kur bir ofise ait değildir, satır güvenliği yoktur.
-- date: TCMB'nin bülteni yayımladığı gün. unit: 1 (JPY gibi bazı dövizlerde
-- 100); 1 birim kur = değer / unit.
CREATE TABLE exchange_rates (
    date              DATE           NOT NULL,
    currency          CHAR(3)        NOT NULL,
    unit              INTEGER        NOT NULL DEFAULT 1,
    forex_buying      NUMERIC(18,6)  NULL,
    forex_selling     NUMERIC(18,6)  NULL,
    banknote_buying   NUMERIC(18,6)  NULL,
    banknote_selling  NUMERIC(18,6)  NULL,
    -- tcmb: TCMB beslemesi; demo: demo verisi.
    source            TEXT           NOT NULL DEFAULT 'tcmb',
    fetched_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (date, currency),
    CONSTRAINT chk_exchange_rates_currency CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_exchange_rates_unit CHECK (unit > 0),
    CONSTRAINT chk_exchange_rates_source CHECK (source IN ('tcmb', 'demo'))
);

CREATE INDEX idx_exchange_rates_currency_date ON exchange_rates (currency, date DESC);

-- Ofis ayarı: faturaya hangi TCMB kuru yazılsın (varsayılan döviz alış) ve
-- otomatik doldurma açık mı. Satırı olmayan ofis: döviz alış, açık.
CREATE TABLE exchange_rate_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    rate_kind        TEXT         NOT NULL DEFAULT 'forex_buying',
    auto_fill        BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_exchange_rate_settings_kind
        CHECK (rate_kind IN ('forex_buying', 'forex_selling', 'banknote_buying', 'banknote_selling'))
);

CREATE INDEX idx_exchange_rate_settings_updated_by ON exchange_rate_settings (updated_by);

SELECT rls_apply_org('exchange_rate_settings');

-- Kurun kaynağı: 'tcmb' otomatik (exchange_rate_date: kullanılan TCMB
-- bülteninin günü), 'manual' ofiste elle girildi. NULL: kaynak belgeden
-- geldi (e-fatura) ya da kur yok. Elle girilen kur otomatikle ezilmez.
ALTER TABLE invoices
    ADD COLUMN exchange_rate_source TEXT NULL,
    ADD COLUMN exchange_rate_date   DATE NULL,
    ADD CONSTRAINT chk_invoices_exchange_rate_source CHECK (exchange_rate_source IN ('tcmb', 'manual'));

UPDATE invoices SET exchange_rate_source = 'manual' WHERE exchange_rate_set_at IS NOT NULL;

ALTER TABLE foreign_service_invoices
    ADD COLUMN exchange_rate_source TEXT NULL,
    ADD COLUMN exchange_rate_date   DATE NULL,
    ADD CONSTRAINT chk_foreign_service_invoices_rate_source CHECK (exchange_rate_source IN ('tcmb', 'manual'));

UPDATE foreign_service_invoices SET exchange_rate_source = 'manual'
WHERE exchange_rate IS NOT NULL AND currency <> 'TRY';
