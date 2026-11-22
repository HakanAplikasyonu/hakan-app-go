-- Mükelleften yabancı hizmet faturası (#780, KDV2 A2): mükellef panelinden
-- yükleme ve WhatsApp'tan "Yabancı hizmet faturası" olarak kaydetme. Dosya
-- attachments deposunda durur (source_uuid = attachments.uuid); satır taslak
-- olarak açılır, satıcı / ülke / tutar boş olabilir (AI okuyamadıysa ofis
-- elle girer). Onay Go'da eksiksiz satır ister.

-- Kaynaktan gelen taslakta tutar 0 ve ülke boş olabilir.
ALTER TABLE foreign_service_invoices DROP CONSTRAINT chk_foreign_service_invoices_amount;
ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_amount
    CHECK (amount > 0 OR (amount = 0 AND status = 'draft' AND source_kind IS NOT NULL));
ALTER TABLE foreign_service_invoices DROP CONSTRAINT chk_foreign_service_invoices_country;
ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_country
    CHECK (country ~ '^[A-Z]{2}$' OR (country = '' AND status = 'draft' AND source_kind IS NOT NULL));

ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('taxpayer_upload', 'whatsapp', 'email'));

-- prefilled: alanlar AI ile önerildi; ofis düzeltip onaylar.
ALTER TABLE foreign_service_invoices ADD COLUMN prefilled BOOLEAN NOT NULL DEFAULT FALSE;

-- Aynı dosya aynı yoldan ikinci kez gelirse yeni satır açılmaz.
CREATE UNIQUE INDEX uq_foreign_service_invoices_source
    ON foreign_service_invoices (organization_id, engagement_id, source_kind, source_uuid)
    WHERE source_uuid IS NOT NULL;

-- WhatsApp uzantısı: yeni hedef "Yabancı hizmet faturası".
ALTER TABLE whatsapp_captures DROP CONSTRAINT chk_whatsapp_captures_target;
ALTER TABLE whatsapp_captures ADD CONSTRAINT chk_whatsapp_captures_target
    CHECK (target IN ('expense', 'income', 'document', 'foreign_invoice'));
