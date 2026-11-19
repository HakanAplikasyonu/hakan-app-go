-- name: UpsertOpenDataWarning :one
-- 05 §1: aynı (varlık, kod) için tek açık uyarı; tekrar üretilirse updated_at ve params güncellenir.
INSERT INTO data_warnings (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id, code, severity, params, producer)
VALUES (@organization_id, @engagement_id, @taxpayer_id, sqlc.narg(period), @entity_type, sqlc.narg(entity_id), @code, @severity, @params, @producer)
ON CONFLICT (engagement_id, entity_type, COALESCE(entity_id, 0), code, COALESCE(period, '')) WHERE status = 'open'
DO UPDATE SET params = EXCLUDED.params, severity = EXCLUDED.severity, updated_at = NOW()
RETURNING *;

-- name: ListDataWarnings :many
SELECT * FROM data_warnings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period))
ORDER BY created_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: ListEntityDataWarnings :many
SELECT * FROM data_warnings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND entity_type = @entity_type AND entity_id = @entity_id
ORDER BY id;

-- name: DecideDataWarning :execrows
UPDATE data_warnings
SET status = @status, resolved_by = sqlc.narg(resolved_by), resolved_at = NOW(), resolution_note = sqlc.narg(note)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'open';
