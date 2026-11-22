ALTER TABLE foreign_service_invoices
    DROP CONSTRAINT IF EXISTS chk_foreign_service_invoices_rate_source,
    DROP COLUMN IF EXISTS exchange_rate_date,
    DROP COLUMN IF EXISTS exchange_rate_source;
ALTER TABLE invoices
    DROP CONSTRAINT IF EXISTS chk_invoices_exchange_rate_source,
    DROP COLUMN IF EXISTS exchange_rate_date,
    DROP COLUMN IF EXISTS exchange_rate_source;
DROP TABLE IF EXISTS exchange_rate_settings;
DROP TABLE IF EXISTS exchange_rates;
