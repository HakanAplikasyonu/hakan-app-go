ALTER TABLE invoices
    DROP COLUMN IF EXISTS counterparty_tax_id_set_at,
    DROP COLUMN IF EXISTS counterparty_tax_id_set_by;
