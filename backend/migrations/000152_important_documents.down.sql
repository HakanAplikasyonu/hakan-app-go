DROP TABLE IF EXISTS important_document_settings;
DROP INDEX IF EXISTS idx_taxpayer_documents_important;
ALTER TABLE taxpayer_documents
    DROP CONSTRAINT IF EXISTS chk_taxpayer_documents_important_kind,
    DROP COLUMN IF EXISTS important_kind;
