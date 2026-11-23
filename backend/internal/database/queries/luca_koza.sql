-- name: GetLucaKozaSettings :one
-- Luca Koza ofis ayarı; satırı yoksa kapalı.
SELECT * FROM luca_koza_settings WHERE organization_id = @organization_id;

-- name: UpsertLucaKozaSettings :one
INSERT INTO luca_koza_settings (organization_id, enabled, updated_by, updated_at)
VALUES (@organization_id, @enabled, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET enabled = EXCLUDED.enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;
