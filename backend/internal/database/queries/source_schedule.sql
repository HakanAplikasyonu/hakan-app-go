-- Zamanlanmış kaynak senkronu (issue #385).

-- tenant-lint: cross-taxpayer worker taraması (rls.System) zamanı gelmiş bağlantıları ofisler arası seçer
-- name: ListDueSourceConnections :many
SELECT taxpayer_id, uuid FROM source_connections
WHERE source_system = @source_system AND setup_state = 'active' AND status IN ('active', 'rate_limited')
  AND sync_interval_minutes IS NOT NULL AND next_sync_at IS NOT NULL AND next_sync_at <= @now
ORDER BY next_sync_at
LIMIT @lim;

-- name: SetSourceConnectionSchedule :one
UPDATE source_connections
SET sync_interval_minutes = sqlc.narg(sync_interval_minutes), next_sync_at = sqlc.narg(next_sync_at), sync_failures = @sync_failures
WHERE taxpayer_id = @taxpayer_id AND id = @id
RETURNING *;
