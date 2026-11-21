DROP TABLE IF EXISTS tax_return_versions;
ALTER TABLE tax_returns DROP CONSTRAINT IF EXISTS chk_tax_returns_version;
ALTER TABLE tax_returns DROP COLUMN IF EXISTS version;
