-- Mükellef eklerken vergi levhasından okunan bilgiler (Oğuz'un geri
-- bildirimi): vergi dairesi, iş yeri adresi ve ana faaliyet kodu (NACE).
-- taxpayers.tax_office zaten vardı; adres ve faaliyet kodu eklenir.
-- Ekleme talebi üçünü de saklar; talep onaylanıp yeni mükellef kimliği
-- açılırken taxpayers'a yazılır. Eski kayıtlar boş kalır.
ALTER TABLE taxpayers
    ADD COLUMN address       VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN activity_code VARCHAR(16)  NOT NULL DEFAULT '';

ALTER TABLE taxpayer_onboarding_requests
    ADD COLUMN tax_office    VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN address       VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN activity_code VARCHAR(16)  NOT NULL DEFAULT '';
