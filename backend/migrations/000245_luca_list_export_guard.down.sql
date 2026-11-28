-- Sahip FORCE altında: veri silen adım sistem kipinde (DATABASE_RULES).
SET LOCAL app.system = 'on';

DELETE FROM expense_export_documents WHERE source_kind <> 'expense';
ALTER TABLE expense_export_documents DROP CONSTRAINT IF EXISTS chk_expense_export_documents_source;
ALTER TABLE expense_export_documents DROP COLUMN IF EXISTS source_uuid;
ALTER TABLE expense_export_documents DROP COLUMN IF EXISTS source_kind;
ALTER TABLE expense_export_documents ALTER COLUMN expense_uuid SET NOT NULL;
