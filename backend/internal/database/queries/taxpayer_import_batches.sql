-- Toplu mükellef aktarımı (issue #273). Ofis kapsamlı.

-- name: CreateTaxpayerImportBatch :one
INSERT INTO taxpayer_import_batches (organization_id, file_name, rows, total, valid, invalid, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(file_name), sqlc.arg(rows), sqlc.arg(total), sqlc.arg(valid),
        sqlc.arg(invalid), sqlc.narg(created_by))
RETURNING *;

-- name: GetTaxpayerImportBatch :one
SELECT * FROM taxpayer_import_batches
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid);

-- name: LockTaxpayerImportBatch :one
SELECT * FROM taxpayer_import_batches
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: ListTaxpayerImportBatches :many
SELECT id, uuid, organization_id, file_name, total, valid, invalid, created_by, created_at, sent_at, sent_by
FROM taxpayer_import_batches
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: MarkTaxpayerImportBatchSent :execrows
UPDATE taxpayer_import_batches
SET sent_at = NOW(), sent_by = sqlc.narg(sent_by), outcome = sqlc.arg(outcome)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND sent_at IS NULL;

-- Kurulum sihirbazının durumu organizations.settings içindeki "setup"
-- anahtarında; ayarların geri kalanına dokunulmaz.

-- name: GetOrganizationSetup :one
SELECT COALESCE(setup_state, '{}'::jsonb)::jsonb AS setup FROM organizations WHERE id = sqlc.arg(organization_id);

-- name: SetOrganizationSetup :one
UPDATE organizations
SET setup_state = sqlc.arg(setup)::jsonb, updated_at = NOW()
WHERE id = sqlc.arg(organization_id)
RETURNING setup_state::jsonb AS setup;

-- name: MarkTaxpayerImportBatchSentOutcome :execrows
UPDATE taxpayer_import_batches
SET outcome = sqlc.arg(outcome)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);
