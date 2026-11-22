-- Yabancı fatura kaynaklı fiş satırlarının kaynak işareti silinir (satırlar
-- kalır). Sahip FORCE altında: veri değiştiren adım sistem kipinde.
SET LOCAL app.system = 'on';
UPDATE journal_draft_lines SET source_kind = NULL, source_id = NULL WHERE source_kind = 'foreign_invoice';
ALTER TABLE journal_draft_lines DROP CONSTRAINT chk_journal_draft_lines_source_kind;
ALTER TABLE journal_draft_lines ADD CONSTRAINT chk_journal_draft_lines_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'invoice_item', 'bank_transaction', 'receipt', 'manual', 'payroll'));

DROP TABLE IF EXISTS foreign_invoice_journal_links;

ALTER TABLE foreign_invoice_settings
    DROP COLUMN IF EXISTS kdv2_account,
    DROP COLUMN IF EXISTS stopaj_account,
    DROP COLUMN IF EXISTS deductible_vat_account,
    DROP COLUMN IF EXISTS counter_account,
    DROP COLUMN IF EXISTS rounding_income_account,
    DROP COLUMN IF EXISTS rounding_expense_account;

ALTER TABLE foreign_service_kind_settings DROP COLUMN IF EXISTS expense_account;
