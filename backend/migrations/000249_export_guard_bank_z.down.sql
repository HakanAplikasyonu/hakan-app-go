-- Sahip FORCE altında: veri silen adım sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

DELETE FROM export_batch_documents WHERE source_kind IN ('bank_transaction', 'retail_sale');
ALTER TABLE export_batch_documents DROP CONSTRAINT chk_export_batch_documents_kind;
ALTER TABLE export_batch_documents ADD CONSTRAINT chk_export_batch_documents_kind
    CHECK (source_kind IN ('invoice', 'expense'));
