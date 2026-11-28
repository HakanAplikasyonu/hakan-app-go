-- Fiş yapay zekâ kontrolleri (Oğuz'un geri bildirimi). Fiş okunurken
-- (receipts usecase) mükellefe çekim önizlemesinde, ofise onay ekranında
-- rozet olarak gösterilir; ofis uyarı ayarlarından kapatılabilir.
-- catalog.go ile aynı; catalog_test.go denetler.
--   RECEIPT_BUYER_MISMATCH: fişteki alıcının VKN/TCKN'si (yoksa unvanı)
--     mükellefinkiyle uyuşmuyor: fiş başka biri adına kesilmiş.
--   PLATE_UNREADABLE: akaryakıt fişinde plaka okunamadı (plaka kontrolü
--     yapılamadı). Üretici control:plate (PLATE_NOT_REGISTERED ile aynı).
INSERT INTO warning_codes (code, section, default_severity, default_params) VALUES
    ('RECEIPT_BUYER_MISMATCH', '3.3', 'warning', '{}'),
    ('PLATE_UNREADABLE',       '3.3', 'warning', '{}');
