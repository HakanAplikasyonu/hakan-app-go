ALTER TABLE bank_transaction_matches
    DROP CONSTRAINT IF EXISTS chk_bank_transaction_matches_fx_rates,
    DROP COLUMN IF EXISTS fx_diff_try,
    DROP COLUMN IF EXISTS fx_rate_payment,
    DROP COLUMN IF EXISTS fx_rate_invoice;
