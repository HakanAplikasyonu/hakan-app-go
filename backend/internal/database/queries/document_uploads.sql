-- Evrak yüklerken tür ve dönem seçimi (Oğuz'un demo geri bildirimi).

-- name: InsertOfficeDocumentUpload :one
INSERT INTO office_document_uploads (
    organization_id, engagement_id, taxpayer_id, doc_type, period, requested_period, period_source,
    document_date, lock_choice, locked_period, ocr_period, status, file_name, content_type, size_bytes,
    sha256, receipt_uuid, upload_job_uuid, archive_document_uuid, error, created_by
) VALUES (
    @organization_id, @engagement_id, @taxpayer_id, @doc_type, @period, @requested_period, @period_source,
    sqlc.narg(document_date), sqlc.narg(lock_choice), sqlc.narg(locked_period), sqlc.narg(ocr_period), @status,
    @file_name, @content_type, @size_bytes, @sha256, sqlc.narg(receipt_uuid), sqlc.narg(upload_job_uuid),
    sqlc.narg(archive_document_uuid), @error, sqlc.narg(created_by)
)
RETURNING *;

-- name: ListOfficeDocumentUploads :many
SELECT d.*, COALESCE(u.name, '')::text AS created_by_name
FROM office_document_uploads d
LEFT JOIN users u ON u.id = d.created_by
WHERE d.organization_id = @organization_id AND d.engagement_id = @engagement_id
  AND (sqlc.narg(period)::text IS NULL OR d.period = sqlc.narg(period)::text)
ORDER BY d.created_at DESC, d.id DESC
LIMIT @lim;

-- name: GetExpenseDraftDate :one
-- Yüklemeden doğan masraf taslağının fotoğraftan okunan tarihi.
SELECT expense_date FROM expenses WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: GetRetailSaleDraftDate :one
-- Yüklemeden doğan Z raporu taslağının fotoğraftan okunan tarihi.
SELECT sale_date FROM retail_sales WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: SetExpenseDraftPeriod :execrows
-- Ofisin seçtiği ay (onay bekleyen taslakta).
UPDATE expenses SET period = @period
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid AND approval_status = 'pending';

-- name: SetRetailSaleDraftPeriod :execrows
-- Ofisin seçtiği ay (onay bekleyen taslakta).
UPDATE retail_sales SET period = @period
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid AND approval_status = 'pending';

-- name: GetDocumentUploadSettings :one
-- Evrak yüklemede tür ve dönem seçimi ofis ayarı; satırı yoksa açık.
SELECT * FROM document_upload_settings WHERE organization_id = @organization_id;

-- name: UpsertDocumentUploadSettings :one
INSERT INTO document_upload_settings (organization_id, enabled, updated_by, updated_at)
VALUES (@organization_id, @enabled, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;
