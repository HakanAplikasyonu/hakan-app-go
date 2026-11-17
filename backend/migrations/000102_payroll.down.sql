-- Bordro verisi (personel, bordro, SGK primleri, bordro belgeleri) silinir.
-- Bordroya ya da SGK'ya yapılmış banka eşleşmeleri ve bordro kaynaklı fiş
-- satırlarının kaynak işareti de silinir (satırlar kalır).
-- Sahip FORCE altında: veri silen adımlar sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';
DELETE FROM bank_transaction_matches WHERE target_type IN ('payroll_run', 'sgk_premium');
ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice'));

UPDATE journal_draft_lines SET source_kind = NULL, source_id = NULL WHERE source_kind = 'payroll';
ALTER TABLE journal_draft_lines DROP CONSTRAINT chk_journal_draft_lines_source_kind;
ALTER TABLE journal_draft_lines ADD CONSTRAINT chk_journal_draft_lines_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('invoice', 'invoice_item', 'bank_transaction', 'receipt', 'manual'));

DROP TABLE IF EXISTS payroll_documents;
DROP TABLE IF EXISTS sgk_premiums;
DROP TABLE IF EXISTS payroll_lines;
DROP TABLE IF EXISTS payroll_runs;
DROP TABLE IF EXISTS employees;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.payroll.read', 'tenant.payroll.write'));
DELETE FROM permissions WHERE slug IN ('tenant.payroll.read', 'tenant.payroll.write');
