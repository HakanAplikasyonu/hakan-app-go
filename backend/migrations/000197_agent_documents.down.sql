-- Sahip FORCE altında: veri silen adımlar sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

UPDATE foreign_service_invoices SET source_kind = 'taxpayer_upload' WHERE source_kind = 'agent';
ALTER TABLE foreign_service_invoices DROP CONSTRAINT chk_foreign_service_invoices_source_kind;
ALTER TABLE foreign_service_invoices ADD CONSTRAINT chk_foreign_service_invoices_source_kind
    CHECK (source_kind IS NULL OR source_kind IN ('taxpayer_upload', 'whatsapp', 'email'));

DROP TABLE IF EXISTS agent_documents;
