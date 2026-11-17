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

-- tenant-lint: cross-taxpayer Easytrade webhook'u şirket kimliğiyle gelir, bağlantının mükellefi bilinmez (rls.System)
-- name: ListSourceConnectionsByExternalCompany :many
SELECT taxpayer_id, uuid FROM source_connections
WHERE source_system = @source_system AND external_company_id = @external_company_id
  AND setup_state = 'active' AND status IN ('active', 'rate_limited')
ORDER BY id;

-- name: PullSourceConnectionNow :one
-- Webhook (issue #385): zamanlanmış bağlantının sıradaki senkronunu şimdiye
-- çeker ve son bildirim zamanını yazar.
UPDATE source_connections
SET next_sync_at = CASE WHEN sync_interval_minutes IS NULL THEN next_sync_at
                        ELSE LEAST(COALESCE(next_sync_at, @now::timestamptz), @now::timestamptz) END,
    meta = meta || jsonb_build_object('last_webhook_at', @now::timestamptz)
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid
RETURNING id;
