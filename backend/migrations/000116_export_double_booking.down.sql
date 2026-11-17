-- Belge izi ve Luca çekim ayarları silinir; çıktı dosyaları ve
-- export_batch_items kalır (fiş ↔ çıktı bağı bozulmaz).
DROP TABLE IF EXISTS export_batch_documents;
ALTER TABLE export_settings DROP CONSTRAINT IF EXISTS chk_export_settings_integrators;
ALTER TABLE export_settings DROP COLUMN IF EXISTS luca_pull_integrators;
DROP TABLE IF EXISTS office_export_settings;
