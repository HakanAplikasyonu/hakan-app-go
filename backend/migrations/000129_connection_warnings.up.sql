-- Bağlantı sağlığı uyarıları (#669 K). Kaynak bağlantısı hata verince ya da
-- uzun süre veri çekmeyince mükellefin uyarılarına ve Komuta Merkezi'ne düşer
-- (05 §3.5). Üretici control:source_connection, varlık source_connection;
-- bağlantı düzelince uyarı kendiliğinden kapanır. catalog.go ile aynı;
-- catalog_test.go denetler.
--   INT_CONNECTION_FAILED: status auth_failed / rate_limited ya da last_error
--     dolu (son deneme hata verdi).
--   INT_SYNC_STALE: kurulmuş bağlantı stale_days gündür başarılı çekim
--     yapmadı; hiç başarılı çekim yapmadıysa kurulumdan never_synced_days gün
--     sonra.
INSERT INTO warning_codes (code, section, default_severity, default_params) VALUES
    ('INT_CONNECTION_FAILED', '3.5', 'blocker', '{}'),
    ('INT_SYNC_STALE',        '3.5', 'warning', '{"stale_days": 3, "never_synced_days": 1}');
