-- Excel içe aktarma (issue #22).

-- name: CreateExcelImport :one
INSERT INTO excel_imports (
    organization_id, engagement_id, taxpayer_id, created_by, file_name, format, sha256, size_bytes,
    storage_key, attachment_id, data_type, source_system, period, status
) VALUES (
    @organization_id, @engagement_id, @taxpayer_id, sqlc.narg(created_by), @file_name, @format, @sha256, @size_bytes,
    @storage_key, sqlc.narg(attachment_id), sqlc.narg(data_type), sqlc.narg(source_system), sqlc.narg(period), @status
)
RETURNING *;

-- name: GetExcelImport :one
SELECT * FROM excel_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: GetExcelImportForUpdate :one
SELECT * FROM excel_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
FOR UPDATE;

-- name: ListExcelImports :many
SELECT * FROM excel_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: CountExcelImports :one
SELECT count(*)::bigint FROM excel_imports
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: UpdateExcelImportAnalysis :one
UPDATE excel_imports
SET status = @status, data_type = sqlc.narg(data_type), source_system = sqlc.narg(source_system),
    sheet_name = sqlc.narg(sheet_name), layout = @layout, mapping = @mapping,
    template_id = sqlc.narg(template_id), ai_used = @ai_used, summary = @summary, error = sqlc.narg(error)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: MarkExcelImportCommitted :one
UPDATE excel_imports
SET status = 'committed', summary = @summary, idempotency_key = @idempotency_key,
    committed_by = sqlc.narg(committed_by), committed_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: MarkExcelImportRolledBack :one
UPDATE excel_imports
SET status = 'rolled_back', summary = @summary, rolled_back_by = sqlc.narg(rolled_back_by), rolled_back_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id
RETURNING *;

-- name: DeleteExcelImportRows :exec
DELETE FROM excel_import_rows
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id;

-- name: InsertExcelImportRow :exec
INSERT INTO excel_import_rows (organization_id, engagement_id, import_id, row_no, raw, record, result, messages, skipped)
VALUES (@organization_id, @engagement_id, @import_id, @row_no, @raw, sqlc.narg(record), @result, @messages, @skipped);

-- name: ListExcelImportRows :many
SELECT * FROM excel_import_rows
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id
  AND (sqlc.narg(result)::text IS NULL OR result = sqlc.narg(result)::text)
ORDER BY row_no
LIMIT @lim OFFSET @off;

-- name: CountExcelImportRows :one
SELECT count(*)::bigint FROM excel_import_rows
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id
  AND (sqlc.narg(result)::text IS NULL OR result = sqlc.narg(result)::text);

-- name: ListAllExcelImportRows :many
SELECT * FROM excel_import_rows
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id
ORDER BY row_no;

-- name: ListSkippedExcelImportRows :many
SELECT row_no FROM excel_import_rows
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id AND skipped;

-- name: SetExcelImportRowSkipped :execrows
UPDATE excel_import_rows SET skipped = @skipped
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id AND row_no = @row_no;

-- name: SetExcelImportRowOutcome :exec
UPDATE excel_import_rows SET invoice_id = sqlc.narg(invoice_id), duplicate = @duplicate
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id AND row_no = @row_no;

-- name: ListExcelImportCreatedInvoices :many
SELECT invoice_id::bigint FROM excel_import_rows
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND import_id = @import_id
  AND invoice_id IS NOT NULL AND NOT duplicate;

-- name: DeleteExcelImportedInvoice :execrows
-- Rollback: yalnızca bu içe aktarmanın yazdığı fatura (capture_method ile korunur).
DELETE FROM invoices
WHERE taxpayer_id = @taxpayer_id AND id = @id AND capture_method = 'excel_import';

-- tenant-lint: cross-org şablon araması kendi ofisi, kendi mükellefi ve global ile sınırlı
-- name: FindExcelImportTemplates :many
-- En özel kapsam önce: company > tenant > global.
SELECT * FROM excel_import_templates
WHERE fingerprint = @fingerprint
  AND (sqlc.narg(data_type)::text IS NULL OR data_type = sqlc.narg(data_type)::text)
  AND ((scope = 'company' AND organization_id = @organization_id AND taxpayer_id = @taxpayer_id)
       OR (scope = 'tenant' AND organization_id = @organization_id)
       OR scope = 'global')
ORDER BY CASE scope WHEN 'company' THEN 0 WHEN 'tenant' THEN 1 ELSE 2 END, updated_at DESC;

-- tenant-lint: cross-org kapsam CHECK ile kısıtlı; company/tenant satırı organization_id taşır
-- name: UpsertExcelImportTemplate :one
INSERT INTO excel_import_templates (scope, organization_id, taxpayer_id, fingerprint, data_type, source_system, header_row, columns, created_by)
VALUES (@scope, sqlc.narg(organization_id), sqlc.narg(taxpayer_id), @fingerprint, @data_type, @source_system, @header_row, @columns, sqlc.narg(created_by))
ON CONFLICT (scope, COALESCE(organization_id, 0), COALESCE(taxpayer_id, 0), fingerprint, data_type)
DO UPDATE SET source_system = EXCLUDED.source_system, header_row = EXCLUDED.header_row,
              columns = EXCLUDED.columns, created_by = EXCLUDED.created_by
RETURNING *;

-- tenant-lint: cross-org ofis görür: kendi şablonları ve global olanlar
-- name: ListExcelImportTemplates :many
SELECT * FROM excel_import_templates
WHERE (organization_id = @organization_id) OR scope = 'global'
ORDER BY scope, updated_at DESC;

-- name: DeleteExcelImportTemplate :execrows
-- Ofis yalnızca kendi şablonunu siler; global şablon platformundur.
DELETE FROM excel_import_templates
WHERE organization_id = @organization_id AND uuid = @uuid AND scope <> 'global';
