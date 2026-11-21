DROP TABLE IF EXISTS operating_account_categories;
DROP TABLE IF EXISTS operating_account_inventories;
ALTER TABLE engagements DROP CONSTRAINT IF EXISTS chk_engagements_ledger_type;
ALTER TABLE engagements DROP COLUMN IF EXISTS ledger_type;
