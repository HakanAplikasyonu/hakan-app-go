-- Müşavir kontrollerinin varsayılan eşiklerini boşaltır (kurallar çalışmaz).
UPDATE warning_codes SET default_params = '{}'::jsonb
WHERE code IN ('FIXED_ASSET_CANDIDATE', 'UNUSUAL_RECORD_TYPE');
