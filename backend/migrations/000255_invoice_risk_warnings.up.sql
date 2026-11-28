-- Fatura risk uyarıları (müşavir toplantısı): olağandışı tutar ve yeni cariye
-- yüksek tutar eşikleri. Ürün kararıdır; ofis / mükellef bazında
-- warning_rule_configs ile değiştirilir.
--
-- UNUSUAL_AMOUNT: fatura, aynı carinin son 12 aydaki faturalarının
--   ortancasının multiplier katını VE min_amount'u aşıyorsa. Carinin en az
--   min_history faturası yoksa mükellefin aynı yöndeki (alış / satış) son
--   12 ayı; o da azsa kural çalışmaz. (Önceki varsayılan: ortalamanın 3 katı,
--   mutlak eşik yok; 10.000 TL'lik bir yerden 31.000 TL gelince uyarıyordu.)
-- NEW_PARTY_HIGH_AMOUNT: ilk kez görülen cari ve tutar limit'in üstünde.
UPDATE warning_codes
SET default_params = '{"multiplier": "10", "min_history": 3, "min_amount": "50000"}'::jsonb
WHERE code = 'UNUSUAL_AMOUNT';

UPDATE warning_codes SET default_params = '{"limit": "100000"}'::jsonb
WHERE code = 'NEW_PARTY_HIGH_AMOUNT';

-- Saatlik tarama son gelen faturaların dönemlerini bulur.
CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices (created_at);
