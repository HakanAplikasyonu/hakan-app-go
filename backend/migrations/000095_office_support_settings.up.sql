-- Destek widget'ı ofis ayarı (issue #387). Portalın ayrı destek sistemi yok;
-- ClickFix Web Widget kullanılır. Mükellefin talebi kendi mali müşavirlik
-- ofisine gider: ofis kendi ClickFix widget kimliğini burada tutar. Satırı
-- olmayan ya da kimliği boş ofiste platform varsayılanı
-- (NEXT_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID) kullanılır.
-- Widget kimliği gizli değildir (sayfaya data-widget olarak yazılır).
CREATE TABLE office_support_settings (
    organization_id    BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    clickfix_widget_id TEXT         NOT NULL DEFAULT '',
    updated_by         BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_office_support_settings_widget CHECK (clickfix_widget_id ~ '^[A-Za-z0-9_-]{0,128}$')
);

-- Ofis politikası + FORCE (rls_apply_org, 000086). Mükellef kapsamı satırı
-- rls.ForOffice ile ofisin kapsamına geçerek okur.
SELECT rls_apply_org('office_support_settings');
