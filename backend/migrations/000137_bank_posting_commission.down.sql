ALTER TABLE bank_transaction_postings
    DROP CONSTRAINT chk_bank_transaction_postings_commission,
    DROP COLUMN commission_account_code,
    DROP COLUMN commission_amount;
