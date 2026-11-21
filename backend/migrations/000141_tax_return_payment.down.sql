ALTER TABLE tax_returns
    DROP CONSTRAINT chk_tax_returns_payment_tx,
    DROP CONSTRAINT chk_tax_returns_paid,
    DROP COLUMN paid_recorded_at,
    DROP COLUMN paid_by,
    DROP COLUMN payment_bank_transaction_id,
    DROP COLUMN paid_on;
