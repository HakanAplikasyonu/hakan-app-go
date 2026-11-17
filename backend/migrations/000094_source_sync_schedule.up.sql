-- Zamanlanmış kaynak senkronu (issue #385). Mükellef başına aralık:
-- sync_interval_minutes NULL ya da next_sync_at NULL ise bağlantı
-- zamanlanmaz (yalnızca elle senkron). Hata sonrası yeniden deneme
-- sync_failures ile üstel geri çekilir (next_sync_at öne çekilir, aralığı
-- geçmez). Kolonlar kaynak bağımsızdır; bugün Easytrade kullanır.
-- source_connections zaten mükellef kapsamlı RLS altında (000046, 000086).
ALTER TABLE source_connections
    ADD COLUMN sync_interval_minutes INTEGER     NULL,
    ADD COLUMN next_sync_at          TIMESTAMPTZ NULL,
    ADD COLUMN sync_failures         INTEGER     NOT NULL DEFAULT 0,
    ADD CONSTRAINT chk_source_connections_sync_interval
        CHECK (sync_interval_minutes IS NULL OR sync_interval_minutes BETWEEN 15 AND 10080),
    ADD CONSTRAINT chk_source_connections_sync_failures CHECK (sync_failures >= 0);

CREATE INDEX idx_source_connections_due ON source_connections (source_system, next_sync_at)
    WHERE next_sync_at IS NOT NULL;
