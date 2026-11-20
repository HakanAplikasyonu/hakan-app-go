-- Bağlantı sağlığı uyarı kodları silinir (bu kodlardaki uyarılar kaybolur).
DELETE FROM data_warnings WHERE code IN ('INT_CONNECTION_FAILED', 'INT_SYNC_STALE');
DELETE FROM warning_codes WHERE code IN ('INT_CONNECTION_FAILED', 'INT_SYNC_STALE');
