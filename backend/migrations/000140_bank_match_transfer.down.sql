DELETE FROM bank_transaction_matches WHERE target_type = 'bank_transaction';
ALTER TABLE bank_transaction_matches DROP CONSTRAINT chk_bank_transaction_matches_target_type;
ALTER TABLE bank_transaction_matches ADD CONSTRAINT chk_bank_transaction_matches_target_type
    CHECK (target_type IN ('invoice', 'payroll_run', 'sgk_premium', 'journal_line', 'financial_transaction'));
