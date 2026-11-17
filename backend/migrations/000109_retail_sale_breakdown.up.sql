-- Z raporu yönetimi (issue #384): KDV oranı ve ödeme tipi kırılımı, Z
-- numarası süreklilik uyarıları. retail_sales'e yalnızca boş bırakılabilir
-- sütunlar eklenir (Easytrade ve diğer kaynaklar eskisi gibi yazar).
ALTER TABLE retail_sales
    ADD COLUMN cash_amount          NUMERIC(18,4) NULL,
    ADD COLUMN card_amount          NUMERIC(18,4) NULL,
    ADD COLUMN other_payment_amount NUMERIC(18,4) NULL,
    ADD COLUMN receipt_count        INT           NULL,
    ADD CONSTRAINT chk_retail_sales_payments CHECK (
        (cash_amount IS NULL OR cash_amount >= 0) AND (card_amount IS NULL OR card_amount >= 0)
        AND (other_payment_amount IS NULL OR other_payment_amount >= 0)
        AND (receipt_count IS NULL OR receipt_count >= 0));

-- ÖKC seri no bazında günlük Z listesi ve süreklilik denetimi.
CREATE INDEX idx_retail_sales_okc ON retail_sales (taxpayer_id, okc_serial_number, sale_date);

-- KDV oranı kırılımı (%0 / %1 / %10 / %20; eski %8 / %18 de kabul).
-- Kaynak veri: mükellefe ait.
CREATE TABLE retail_sale_vat_lines (
    id             BIGSERIAL      PRIMARY KEY,
    taxpayer_id    BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    retail_sale_id BIGINT         NOT NULL,
    tax_rate       NUMERIC(5,2)   NOT NULL,
    tax_base       NUMERIC(18,4)  NOT NULL,
    tax_amount     NUMERIC(18,4)  NOT NULL,
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_retail_sale_vat_lines_rate UNIQUE (taxpayer_id, retail_sale_id, tax_rate),
    CONSTRAINT fk_retail_sale_vat_lines_sale
        FOREIGN KEY (taxpayer_id, retail_sale_id) REFERENCES retail_sales (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_retail_sale_vat_lines_rate CHECK (tax_rate >= 0 AND tax_rate <= 100),
    CONSTRAINT chk_retail_sale_vat_lines_amounts CHECK (tax_base >= 0 AND tax_amount >= 0)
);

SELECT rls_apply_taxpayer('retail_sale_vat_lines'::regclass);

-- Uyarı kodları (catalog.go ile aynı; catalog_test.go denetler).
--   Z_NUMBER_GAP: aynı ÖKC'de art arda iki Z raporu arasında atlanan numara.
--   Z_NUMBER_DUPLICATE: aynı ÖKC'de aynı Z numarası iki farklı günde.
--   Z_DAY_GAP: aynı ÖKC'de iki Z raporu arasında max_gap_days günden uzun
--     boşluk (iş günü takvimi bilinmediği için bilgi düzeyinde).
INSERT INTO warning_codes (code, section, default_severity, default_params) VALUES
    ('Z_NUMBER_GAP',       '3.2', 'warning', '{}'),
    ('Z_NUMBER_DUPLICATE', '3.2', 'warning', '{}'),
    ('Z_DAY_GAP',          '3.2', 'info',    '{"max_gap_days": 1}');
