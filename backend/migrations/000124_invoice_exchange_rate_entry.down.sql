ALTER TABLE invoices
    DROP COLUMN IF EXISTS exchange_rate_set_at,
    DROP COLUMN IF EXISTS exchange_rate_set_by;
