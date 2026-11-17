-- Z raporlarının KDV / ödeme kırılımı ve süreklilik uyarı kodları silinir
-- (kırılım verisi ve bu kodlardaki açık uyarılar kaybolur).
DELETE FROM data_warnings WHERE code IN ('Z_NUMBER_GAP', 'Z_NUMBER_DUPLICATE', 'Z_DAY_GAP');
DELETE FROM warning_codes WHERE code IN ('Z_NUMBER_GAP', 'Z_NUMBER_DUPLICATE', 'Z_DAY_GAP');
DROP TABLE IF EXISTS retail_sale_vat_lines;
DROP INDEX IF EXISTS idx_retail_sales_okc;
ALTER TABLE retail_sales
    DROP CONSTRAINT IF EXISTS chk_retail_sales_payments,
    DROP COLUMN IF EXISTS receipt_count,
    DROP COLUMN IF EXISTS other_payment_amount,
    DROP COLUMN IF EXISTS card_amount,
    DROP COLUMN IF EXISTS cash_amount;
