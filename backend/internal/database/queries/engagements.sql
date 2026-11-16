-- Engagements (issue #2): ofis <-> mükellef hizmet ilişkisi ve üye atamaları.

-- name: GetTaxpayerByUUID :one
SELECT * FROM taxpayers WHERE uuid = $1;

-- name: CreateEngagement :one
INSERT INTO engagements (organization_id, taxpayer_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(taxpayer_id))
RETURNING *;

-- name: GetEngagementByUUID :one
SELECT e.*, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid);

-- name: ListEngagementsByOrg :many
SELECT e.*, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status))
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountEngagementsByOrg :one
SELECT COUNT(*)::bigint
FROM engagements e
WHERE e.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status));

-- name: ListAssignedEngagements :many
SELECT e.*, t.uuid AS taxpayer_uuid, t.title AS taxpayer_title
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
INNER JOIN engagement_assignments a
    ON a.engagement_id = e.id AND a.organization_id = e.organization_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND a.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status))
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAssignedEngagements :one
SELECT COUNT(*)::bigint
FROM engagements e
INNER JOIN engagement_assignments a
    ON a.engagement_id = e.id AND a.organization_id = e.organization_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND a.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(status)::text IS NULL OR e.status = sqlc.narg(status));

-- name: EndEngagement :one
UPDATE engagements
SET status = 'ended', ended_at = NOW()
WHERE organization_id = sqlc.arg(organization_id)
  AND uuid = sqlc.arg(uuid)
  AND ended_at IS NULL
RETURNING *;

-- name: ResolveEngagementAccess :one
SELECT e.id, e.uuid, e.organization_id, e.taxpayer_id, e.status, e.started_at, e.ended_at,
       e.archive_until,
       t.uuid AS taxpayer_uuid,
       EXISTS(
           SELECT 1 FROM engagement_assignments a
           WHERE a.organization_id = e.organization_id
             AND a.engagement_id = e.id
             AND a.user_id = sqlc.arg(user_id)
       )::boolean AS assigned
FROM engagements e
INNER JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.uuid = sqlc.arg(uuid);

-- name: AddEngagementAssignment :exec
INSERT INTO engagement_assignments (organization_id, engagement_id, user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(engagement_id), sqlc.arg(user_id))
ON CONFLICT (engagement_id, user_id) DO NOTHING;

-- name: DeleteEngagementAssignment :execrows
DELETE FROM engagement_assignments
WHERE organization_id = sqlc.arg(organization_id)
  AND engagement_id = sqlc.arg(engagement_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListEngagementAssignments :many
SELECT a.created_at, u.uuid AS user_uuid, u.email, u.name, u.surname
FROM engagement_assignments a
INNER JOIN users u ON u.id = a.user_id AND u.deleted_at IS NULL
WHERE a.organization_id = sqlc.arg(organization_id)
  AND a.engagement_id = sqlc.arg(engagement_id)
ORDER BY a.created_at, a.id;
