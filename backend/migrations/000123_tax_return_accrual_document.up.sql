-- Tahakkuk fişi (#575): beyanname GİB'e verildikten sonra alınan tahakkuk
-- PDF'i beyannamenin kaydına eklenir. Dosya nesne deposunda durur; satır
-- yalnızca anahtarı, adı, boyutu ve özetini tutar. Yalnızca verilmiş
-- (filed) beyana eklenebilir.
ALTER TABLE tax_returns
    ADD COLUMN accrual_storage_key TEXT        NULL,
    ADD COLUMN accrual_file_name   TEXT        NULL,
    ADD COLUMN accrual_size_bytes  BIGINT      NULL,
    ADD COLUMN accrual_sha256      TEXT        NULL,
    ADD COLUMN accrual_uploaded_by BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN accrual_uploaded_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT chk_tax_returns_accrual_document
        CHECK (accrual_storage_key IS NULL OR (status = 'filed' AND accrual_file_name IS NOT NULL
               AND accrual_size_bytes IS NOT NULL AND accrual_sha256 IS NOT NULL AND accrual_uploaded_at IS NOT NULL));
