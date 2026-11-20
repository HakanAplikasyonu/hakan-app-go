ALTER TABLE tax_returns
    DROP CONSTRAINT IF EXISTS chk_tax_returns_accrual_document,
    DROP COLUMN IF EXISTS accrual_uploaded_at,
    DROP COLUMN IF EXISTS accrual_uploaded_by,
    DROP COLUMN IF EXISTS accrual_sha256,
    DROP COLUMN IF EXISTS accrual_size_bytes,
    DROP COLUMN IF EXISTS accrual_file_name,
    DROP COLUMN IF EXISTS accrual_storage_key;
