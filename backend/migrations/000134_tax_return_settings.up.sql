-- Beyan son kontrolü ofis ayarı (#710, ofis kararı A.5): "kontrol edildi" ve
-- "verildi" işaretini yalnız sorumlu koyar (#706); varsayılan açık, ofis
-- kapatabilir. Kapalıyken beyanı hazırlayan (tenant.tax_returns.write) da
-- işaretler; kim / ne zaman yine kaydedilir. Satırı olmayan ofis açık sayılır.
CREATE TABLE tax_return_settings (
    organization_id      BIGINT       PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    final_check_required BOOLEAN      NOT NULL DEFAULT TRUE,
    updated_by           BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tax_return_settings_updated_by ON tax_return_settings (updated_by);

SELECT rls_apply_org('tax_return_settings');
