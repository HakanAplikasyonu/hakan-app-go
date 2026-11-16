DROP INDEX IF EXISTS uq_receipt_captures_client_key;
ALTER TABLE receipt_captures DROP CONSTRAINT IF EXISTS chk_receipt_captures_client_key;
ALTER TABLE receipt_captures DROP COLUMN IF EXISTS client_key;
