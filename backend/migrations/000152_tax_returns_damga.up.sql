-- #575: damga vergisi beyannamesi (MUHSGK vermeyenler; satırlar elle girilir) ayrı beyan türü.
ALTER TABLE tax_returns DROP CONSTRAINT chk_tax_returns_kind;
ALTER TABLE tax_returns ADD CONSTRAINT chk_tax_returns_kind CHECK (kind IN ('kdv1', 'kdv2', 'damga', 'muhsgk', 'babs'));
