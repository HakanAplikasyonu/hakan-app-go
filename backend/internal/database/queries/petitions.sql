-- Dilekçeler (issue #56).

-- name: CreatePetition :one
INSERT INTO petitions (organization_id, engagement_id, template_key, template_version, fields, rendered, draft, created_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(engagement_id), sqlc.arg(template_key), sqlc.arg(template_version),
        sqlc.arg(fields), sqlc.arg(rendered), sqlc.arg(draft), sqlc.narg(created_by))
RETURNING *;

-- name: ListPetitions :many
SELECT p.uuid, p.template_key, p.template_version, p.draft, p.created_at, p.rendered,
       u.name AS creator_name, u.surname AS creator_surname
FROM petitions p
LEFT JOIN users u ON u.id = p.created_by
WHERE p.organization_id = sqlc.arg(organization_id) AND p.engagement_id = sqlc.arg(engagement_id)
ORDER BY p.created_at DESC, p.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: GetPetition :one
SELECT * FROM petitions
WHERE organization_id = sqlc.arg(organization_id) AND engagement_id = sqlc.arg(engagement_id) AND uuid = sqlc.arg(uuid);

-- name: GetPetitionContext :one
SELECT t.title, t.tax_id, t.tax_office, o.name AS office_name
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
INNER JOIN organizations o ON o.id = e.organization_id
WHERE e.organization_id = sqlc.arg(organization_id) AND e.id = sqlc.arg(engagement_id);
