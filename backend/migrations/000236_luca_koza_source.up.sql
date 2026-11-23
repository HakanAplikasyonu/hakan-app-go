-- Luca Koza kaynağı (Oğuz'un geri bildirimi): mükellef programı Luca
-- Koza'dan faturalar YALNIZCA OKUNUR. Şifre hiçbir yerde saklanmaz
-- (credentials_ref boş kalır); bağlantı satırında yalnız adres, kurum kodu,
-- kullanıcı adı ve seçilen şirket/şube durur. Her eşitlemede şifre yeniden
-- istenir.
ALTER TABLE source_connections DROP CONSTRAINT chk_source_connections_source_system;
ALTER TABLE source_connections ADD CONSTRAINT chk_source_connections_source_system
    CHECK (source_system ~ '^(easytrade|parasut|kolaybi|luca_koza|bizimhesap|logo_isbasi|logo_tiger|logo_go|mikro|robom|manual_upload|entegrator_[a-z0-9_]+|bank_[a-z0-9_]+)$');

-- Ofis ayarı: Luca Koza bağlantısı açık/kapalı. Satırı yoksa KAPALI;
-- kapalıyken bağlantı uçları 409 döner ve kart "Yakında" görünür.
CREATE TABLE luca_koza_settings (
    organization_id  BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    enabled          BOOLEAN      NOT NULL DEFAULT FALSE,
    updated_by       BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_luca_koza_settings_updated_by ON luca_koza_settings (updated_by);

SELECT rls_apply_org('luca_koza_settings');
