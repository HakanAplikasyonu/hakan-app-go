-- KDV nöbetçisi bildirim kanalı (#780 B, demo geri bildirimi): ofis
-- nöbetçinin uyarılarını yalnız uygulama içinde mi görsün, yoksa mükellefin
-- sorumlusuna e-posta da mı gitsin? Varsayılan yalnız uygulama içi. Eşik
-- tutarı burada tutulmaz: warning_rule_configs (VAT_PAYABLE_THRESHOLD.limit)
-- nöbetçinin ve uyarı kuralının okuduğu tek yerdir.
ALTER TABLE vat_tracking_settings ADD COLUMN watch_email BOOLEAN NOT NULL DEFAULT FALSE;
