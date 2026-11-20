UPDATE bank_transactions SET is_cancelled = FALSE WHERE excluded_at IS NOT NULL;
ALTER TABLE bank_transactions
    DROP COLUMN excluded_at,
    DROP COLUMN excluded_by;
