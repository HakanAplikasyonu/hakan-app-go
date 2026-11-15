-- Müşavir kontrolleri (issue #31, 06 T3): demirbaş ve olağandışı kayıt türü
-- için varsayılan eşikler. Ürün kararıdır; ofis / mükellef bazında
-- warning_rule_configs ile değiştirilir.
--
-- FIXED_ASSET_CANDIDATE: gelen faturada kalem adı anahtar kelimelerden
--   biriyle başlayan bir sözcük içeriyorsa ya da KDV hariç tutar
--   amount_limit'i aşıyorsa. amount_limit varsayılanı boş: doğrudan gider
--   yazma sınırını müşavir girer.
-- UNUSUAL_RECORD_TYPE: kalem adının sözcüklerinden en az min_overlap kadarı
--   mükellefin son 12 ayda aynı yöndeki kalem adlarında geçmiyorsa
--   (geçmişte en az min_history kalem satırı varken).
UPDATE warning_codes SET default_params = jsonb_build_object(
    'keywords', jsonb_build_array(
        'bilgisayar', 'dizüstü', 'laptop', 'notebook', 'sunucu', 'server', 'monitör', 'yazıcı',
        'printer', 'tablet', 'telefon', 'cep telefonu', 'mobilya', 'masa', 'sandalye', 'koltuk',
        'dolap', 'klima', 'jeneratör', 'makine', 'makina', 'forklift', 'otomobil', 'binek',
        'kamyonet', 'araç', 'projeksiyon', 'kamera', 'televizyon', 'kompresör'),
    'amount_limit', NULL)
WHERE code = 'FIXED_ASSET_CANDIDATE';

UPDATE warning_codes SET default_params = '{"min_history": 12, "min_overlap": "0.34"}'::jsonb
WHERE code = 'UNUSUAL_RECORD_TYPE';
