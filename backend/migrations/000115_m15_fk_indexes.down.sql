-- Yalnızca indeksler; veri kaybı yok.
DROP INDEX IF EXISTS idx_whatsapp_captures_task;
DROP INDEX IF EXISTS idx_whatsapp_captures_receipt;
DROP INDEX IF EXISTS idx_whatsapp_captures_attachment;
DROP INDEX IF EXISTS idx_reconciliation_runs_previous;
DROP INDEX IF EXISTS idx_reconciliation_runs_import;
DROP INDEX IF EXISTS idx_payroll_documents_run;
DROP INDEX IF EXISTS idx_payroll_documents_employee;
DROP INDEX IF EXISTS idx_payroll_runs_journal;
DROP INDEX IF EXISTS idx_receipt_postings_journal;
DROP INDEX IF EXISTS idx_sgk_premiums_bank_transaction;
DROP INDEX IF EXISTS idx_sgk_premiums_run;
DROP INDEX IF EXISTS idx_accounting_transfers_agent;
DROP INDEX IF EXISTS idx_accounting_transfers_batch;
DROP INDEX IF EXISTS idx_bank_ledger_accounts_cash_box;
DROP INDEX IF EXISTS idx_bank_transaction_postings_transaction;
