-- Fiş arka planda okunur (#780): mobil uygulama fotoğrafı gönderince sunucu
-- fotoğrafı saklayıp yakalamayı 'processing' ile açar ve hemen 202 döner;
-- yapay zekâ okuması arka planda biter ve kaydı draft / duplicate / failed
-- yapar. Okuma isteğin içinde bittiğinde istek uzuyor, telefon bekleyemeyip
-- fişi "Bağlantı bekleniyor"da bırakıyordu.
ALTER TABLE receipt_captures DROP CONSTRAINT chk_receipt_captures_status;
ALTER TABLE receipt_captures ADD CONSTRAINT chk_receipt_captures_status
    CHECK (status IN ('processing', 'draft', 'duplicate', 'failed', 'approved', 'rejected'));
