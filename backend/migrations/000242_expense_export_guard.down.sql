-- Sahip FORCE altında: veri silen adım sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

DROP TABLE IF EXISTS expense_export_documents;

DELETE FROM export_batch_documents WHERE source_kind = 'expense';
ALTER TABLE export_batch_documents DROP COLUMN IF EXISTS expense_uuid;
ALTER TABLE export_batch_documents DROP CONSTRAINT chk_export_batch_documents_kind;
ALTER TABLE export_batch_documents ADD CONSTRAINT chk_export_batch_documents_kind CHECK (source_kind IN ('invoice'));
