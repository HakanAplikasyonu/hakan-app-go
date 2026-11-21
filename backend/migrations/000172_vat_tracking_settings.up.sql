-- KDV takibi ofis ayarları (#780 B, 21.11 ofis görüşmesi): ay sonu KDV
-- raporu (tüm mükellefler; alınan, satılan, devreden) açık/kapalı ve hangi
-- günden itibaren "hazır" gösterileceği (varsayılan ayın 29'u; ay kısaysa
-- son gün). Satırı olmayan ofis varsayılanları kullanır.
CREATE TABLE vat_tracking_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    report_enabled   BOOLEAN      NOT NULL DEFAULT TRUE,
    report_day       INTEGER      NOT NULL DEFAULT 29,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_vat_tracking_settings_day CHECK (report_day BETWEEN 1 AND 31)
);

CREATE INDEX idx_vat_tracking_settings_updated_by ON vat_tracking_settings (updated_by);

SELECT rls_apply_org('vat_tracking_settings');
