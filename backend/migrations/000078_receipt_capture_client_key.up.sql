-- Mükellef fiş yüklemesi (#199): mobil uygulama her fotoğrafı Idempotency-Key
-- ile gönderir. Aynı anahtarla tekrar gelen yükleme (bağlantı koptu, yanıt
-- kayboldu) yeni kayıt açmaz, ilk kaydı döner.
ALTER TABLE receipt_captures ADD COLUMN client_key TEXT NULL;
ALTER TABLE receipt_captures ADD CONSTRAINT chk_receipt_captures_client_key
    CHECK (client_key IS NULL OR (length(client_key) BETWEEN 1 AND 128));
CREATE UNIQUE INDEX uq_receipt_captures_client_key
    ON receipt_captures (taxpayer_id, client_key) WHERE client_key IS NOT NULL;
