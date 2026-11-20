ALTER TABLE invoices
    DROP CONSTRAINT IF EXISTS chk_invoices_vat_deduction_period,
    DROP COLUMN IF EXISTS vat_deduction_set_at,
    DROP COLUMN IF EXISTS vat_deduction_set_by,
    DROP COLUMN IF EXISTS vat_deduction_period;
