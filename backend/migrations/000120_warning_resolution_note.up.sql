-- Uyarı kararının gerekçesi (#529): "Gördüm" ya da "Çözüldü" diyen kişi
-- (resolved_by) ve zaman (resolved_at) zaten satırda; neden kapattığı
-- da burada tutulur. Toplu onayın notu da buraya yazılır.
ALTER TABLE data_warnings ADD COLUMN resolution_note TEXT NULL;
ALTER TABLE data_warnings ADD CONSTRAINT chk_data_warnings_resolution_note
    CHECK (resolution_note IS NULL OR char_length(resolution_note) <= 500);
