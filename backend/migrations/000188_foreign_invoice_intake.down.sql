-- Sahip FORCE altında: veri silen adımlar sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

DELETE FROM whatsapp_captures WHERE target = 'foreign_invoice';
ALTER TABLE whatsapp_captures DROP CONSTRAINT chk_whatsapp_captures_target;
ALTER TABLE whatsapp_captures ADD CONSTRAINT chk_whatsapp_captures_target
    CHECK (target IN ('expense', 'income', 'document'));

DROP INDEX IF EXISTS uq_foreign_service_invoices_source;
ALTER TABLE foreign_service_invoices DROP COLUMN IF EXISTS prefilled;
ALTER TABLE foreign_service_invoices DROP CONSTRAINT IF EXISTS chk_foreign_service_invoices_source_kind;
DELETE FROM foreign_service_invoices WHERE amount = 0 OR country = '';
ALTER TABLE foreign_service_invoices DROP CONSTRAINT chk_foreign_service_invoices_country;
ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_country CHECK (country ~ '^[A-Z]{2}$');
ALTER TABLE foreign_service_invoices DROP CONSTRAINT chk_foreign_service_invoices_amount;
ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_amount CHECK (amount > 0);
