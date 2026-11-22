-- name: InsertChangeRequest :one
INSERT INTO change_requests (organization_id, engagement_id, taxpayer_id, period, entity_type, entity_id, old_hash, new_hash, diff, new_payload)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, @entity_type, sqlc.narg(entity_id), sqlc.narg(old_hash), sqlc.narg(new_hash), @diff, @new_payload)
RETURNING *;

-- name: ListOpenChangeRequests :many
SELECT * FROM change_requests
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND status = 'open'
ORDER BY created_at, id;

-- name: DecideChangeRequest :execrows
UPDATE change_requests
SET status = @status, decided_by = @decided_by, decided_at = NOW(),
    decision = @decision, target_period = sqlc.narg(target_period)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid AND status = 'open';
