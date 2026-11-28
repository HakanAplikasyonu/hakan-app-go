-- BNK_POS_VS_SALES varsayılanlarını 000039'daki hâline döndürür.
UPDATE warning_codes SET default_params = '{"tolerance_days": 3}'::jsonb
WHERE code = 'BNK_POS_VS_SALES';
