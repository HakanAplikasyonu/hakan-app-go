-- Z raporu ↔ banka POS kontrolü (BNK_POS_VS_SALES) varsayılan eşikleri.
-- Ayın Z raporlarındaki kartla tahsilat toplamı, bankaya giren POS
-- tahsilatlarının brüt toplamıyla (net + harekete girilen komisyon)
-- karşılaştırılır. Banka tarafı tolerance_days gün ileri kaydırılır (valör:
-- banka günün satışını ertesi gün öder). Fark min_amount TL ile Z toplamının
-- yüzde tolerance_percent'inden büyük olanı aşarsa uyarı çıkar. Ofis /
-- mükellef bazında warning_rule_configs ile değiştirilir.
UPDATE warning_codes
SET default_params = '{"tolerance_days": 1, "min_amount": 100, "tolerance_percent": 1}'::jsonb
WHERE code = 'BNK_POS_VS_SALES';
