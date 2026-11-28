-- Okuması yarıda kalan yakalamalar okunamayan fiş olur (ofis yenisini ister).
UPDATE receipt_captures SET status = 'failed', error = COALESCE(error, 'receipt read did not finish')
WHERE status = 'processing';
ALTER TABLE receipt_captures DROP CONSTRAINT chk_receipt_captures_status;
ALTER TABLE receipt_captures ADD CONSTRAINT chk_receipt_captures_status
    CHECK (status IN ('draft', 'duplicate', 'failed', 'approved', 'rejected'));
