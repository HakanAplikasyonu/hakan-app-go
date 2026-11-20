-- name: InsertActivityEvent :one
INSERT INTO activity_events (actor_user_id, action, resource, resource_uuid, payload, ip_address, user_agent, organization_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, sqlc.narg(organization_id))
RETURNING *;

-- name: ListActivityEvents :many
SELECT * FROM activity_events
WHERE (sqlc.narg(actor_user_id)::bigint IS NULL OR actor_user_id = sqlc.narg(actor_user_id))
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
  AND (
    sqlc.narg(q)::text IS NULL
    OR action ILIKE '%' || sqlc.narg(q) || '%'
    OR resource ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountActivityEvents :one
SELECT COUNT(*)::bigint FROM activity_events
WHERE (sqlc.narg(actor_user_id)::bigint IS NULL OR actor_user_id = sqlc.narg(actor_user_id))
  AND (sqlc.narg(resource)::text IS NULL OR resource = sqlc.narg(resource))
  AND (sqlc.narg(action)::text IS NULL OR action = sqlc.narg(action))
  AND (
    sqlc.narg(q)::text IS NULL
    OR action ILIKE '%' || sqlc.narg(q) || '%'
    OR resource ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListResourceActivityForEngagement :many
-- Audit history of one record in one engagement (#565): the organization's
-- events about the resource whose payload names the engagement.
SELECT e.uuid, e.action, e.payload, e.created_at,
       u.uuid AS actor_uuid, u.name AS actor_name, u.surname AS actor_surname
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.organization_id = sqlc.arg(organization_id)
  AND e.resource = sqlc.arg(resource)
  AND e.resource_uuid = sqlc.arg(resource_uuid)
  AND lower(e.payload->>'engagement_uuid') = lower(sqlc.arg(engagement_uuid)::text)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit_count);
