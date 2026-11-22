-- Canlı demo (#780): mobilden çekilen masraf fişinin KDV oranı kırılımı
-- (%1 / %10 / %20; eski %8 / %18 de kabul) ve mükellefin son kullandığı
-- hesap kodu önerisi.
--
-- Kaynak veri: mükellefe ait (retail_sale_vat_lines ile aynı biçim). Bir
-- oranın KDV'si okunamadıysa satır yazılmaz; ofis ekranı alanı "kontrol et"
-- diye işaretler (değer uydurulmaz).
CREATE TABLE expense_vat_lines (
    id          BIGSERIAL      PRIMARY KEY,
    taxpayer_id BIGINT         NOT NULL REFERENCES taxpayers (id) ON DELETE RESTRICT,
    expense_id  BIGINT         NOT NULL,
    tax_rate    NUMERIC(5,2)   NOT NULL,
    tax_base    NUMERIC(18,4)  NOT NULL,
    tax_amount  NUMERIC(18,4)  NOT NULL,
    created_at  TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expense_vat_lines_rate UNIQUE (taxpayer_id, expense_id, tax_rate),
    CONSTRAINT fk_expense_vat_lines_expense
        FOREIGN KEY (taxpayer_id, expense_id) REFERENCES expenses (taxpayer_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_expense_vat_lines_rate CHECK (tax_rate >= 0 AND tax_rate <= 100),
    CONSTRAINT chk_expense_vat_lines_amounts CHECK (tax_base >= 0 AND tax_amount >= 0)
);

SELECT rls_apply_taxpayer('expense_vat_lines'::regclass);

-- Hesap önerisinin yeni kaynağı: satıcının geçmişi yoksa mükellefin son
-- onaylı masrafında kullanılan hesap (taxpayer_memory).
ALTER TABLE receipt_postings DROP CONSTRAINT chk_receipt_postings_source;
ALTER TABLE receipt_postings ADD CONSTRAINT chk_receipt_postings_source
    CHECK (account_source IS NULL OR account_source IN ('category', 'vendor_memory', 'taxpayer_memory', 'plan', 'manual'));
