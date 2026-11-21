DELETE FROM tax_returns WHERE kind = 'damga';
ALTER TABLE tax_returns DROP CONSTRAINT chk_tax_returns_kind;
ALTER TABLE tax_returns ADD CONSTRAINT chk_tax_returns_kind CHECK (kind IN ('kdv1', 'kdv2', 'muhsgk', 'babs'));
