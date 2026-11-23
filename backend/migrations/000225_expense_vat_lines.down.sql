-- Sahip FORCE altında: veri silen adımlar sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

UPDATE receipt_postings SET account_source = 'category' WHERE account_source = 'taxpayer_memory';
ALTER TABLE receipt_postings DROP CONSTRAINT chk_receipt_postings_source;
ALTER TABLE receipt_postings ADD CONSTRAINT chk_receipt_postings_source
    CHECK (account_source IS NULL OR account_source IN ('category', 'vendor_memory', 'plan', 'manual'));

DROP TABLE IF EXISTS expense_vat_lines;
