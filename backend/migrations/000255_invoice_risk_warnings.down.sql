DROP INDEX IF EXISTS idx_invoices_created_at;

UPDATE warning_codes SET default_params = '{"limit": "50000"}'::jsonb
WHERE code = 'NEW_PARTY_HIGH_AMOUNT';

UPDATE warning_codes SET default_params = '{"multiplier": "3", "min_history": 3}'::jsonb
WHERE code = 'UNUSUAL_AMOUNT';
