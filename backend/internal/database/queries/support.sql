-- Destek widget'ı ofis ayarı (issue #387).

-- name: GetOfficeSupportSettings :one
SELECT * FROM office_support_settings WHERE organization_id = @organization_id;

-- name: UpsertOfficeSupportSettings :one
INSERT INTO office_support_settings (organization_id, clickfix_widget_id, updated_by, updated_at)
VALUES (@organization_id, @clickfix_widget_id, sqlc.narg(updated_by), NOW())
ON CONFLICT (organization_id) DO UPDATE
SET clickfix_widget_id = EXCLUDED.clickfix_widget_id,
    updated_by = EXCLUDED.updated_by, updated_at = NOW()
RETURNING *;
