-- name: CreateUBLUploadJob :one
INSERT INTO ubl_upload_jobs (organization_id, engagement_id, taxpayer_id, actor_id, file_name, file_key, sha256, size_bytes)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @actor_id, @file_name, @file_key, @sha256, @size_bytes)
RETURNING *;

-- name: GetUBLUploadJob :one
SELECT * FROM ubl_upload_jobs
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: ListUBLUploadJobs :many
SELECT * FROM ubl_upload_jobs
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(period)::text IS NULL OR to_char(created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM') = sqlc.narg(period))
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: cross-org worker loads the job by id; the row carries its own scope
-- name: ClaimUBLUploadJob :one
UPDATE ubl_upload_jobs
SET status = 'processing', started_at = NOW(), error = NULL
WHERE id = @id AND status IN ('queued', 'failed')
RETURNING *;

-- tenant-lint: cross-org worker finishes the job it claimed by id
-- name: FinishUBLUploadJob :exec
UPDATE ubl_upload_jobs
SET status = @status, report = @report, error = sqlc.narg(error), finished_at = NOW()
WHERE id = @id;

-- name: ResolveWarningSetting :one
-- Etkin seviye ve açık/kapalı (#13): engagement ayarı > ofis ayarı > katalog varsayılanı.
SELECT
    COALESCE(e.severity, o.severity, c.default_severity, @fallback::text)::text AS severity,
    COALESCE(e.enabled, o.enabled, TRUE)::boolean AS enabled
FROM (SELECT 1) AS one
LEFT JOIN warning_codes c ON c.code = @code
LEFT JOIN warning_rule_configs o ON o.organization_id = @organization_id AND o.engagement_id IS NULL AND o.code = @code
LEFT JOIN warning_rule_configs e ON e.organization_id = @organization_id AND e.engagement_id = @engagement_id AND e.code = @code;
