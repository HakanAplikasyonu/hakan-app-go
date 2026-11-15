-- name: UpsertSyncWindow :one
INSERT INTO sync_windows (taxpayer_id, connection_id, item, direction, date_field, range_start, range_end)
VALUES (@taxpayer_id, @connection_id, @item, sqlc.narg(direction), @date_field, @range_start, @range_end)
ON CONFLICT (connection_id, item, date_field, range_start, range_end) DO UPDATE SET updated_at = NOW()
RETURNING *;

-- name: ListPendingSyncWindows :many
SELECT * FROM sync_windows
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id AND status IN ('pending', 'failed')
ORDER BY range_start;

-- name: SetSyncWindowStatus :execrows
UPDATE sync_windows
SET status = @status, page_cursor = sqlc.narg(page_cursor), last_error = sqlc.narg(last_error),
    attempts = attempts + CASE WHEN @status::text = 'running' THEN 1 ELSE 0 END,
    started_at = CASE WHEN @status::text = 'running' THEN NOW() ELSE started_at END,
    finished_at = CASE WHEN @status::text IN ('done', 'failed') THEN NOW() ELSE NULL END
WHERE taxpayer_id = @taxpayer_id AND id = @id;

-- name: StartSyncRun :one
INSERT INTO sync_runs (taxpayer_id, connection_id, kind)
VALUES (@taxpayer_id, @connection_id, @kind)
RETURNING *;

-- name: FinishSyncRun :execrows
UPDATE sync_runs
SET status = @status, counters = @counters, error = sqlc.narg(error), finished_at = NOW()
WHERE taxpayer_id = @taxpayer_id AND id = @id AND status = 'running';

-- name: ListSyncRuns :many
SELECT * FROM sync_runs
WHERE taxpayer_id = @taxpayer_id AND connection_id = @connection_id
ORDER BY started_at DESC
LIMIT @lim;
