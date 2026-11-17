-- Zamanlanmış mükellef raporu (issue #386).

-- name: GetReportSchedule :one
SELECT * FROM report_schedules
WHERE organization_id = @organization_id AND engagement_id = @engagement_id;

-- name: UpsertReportSchedule :one
INSERT INTO report_schedules (organization_id, engagement_id, taxpayer_id, enabled, day_of_month, recipient, updated_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @enabled, @day_of_month, sqlc.narg(recipient), sqlc.narg(updated_by))
ON CONFLICT (organization_id, engagement_id) DO UPDATE
SET enabled = EXCLUDED.enabled, day_of_month = EXCLUDED.day_of_month,
    recipient = EXCLUDED.recipient, updated_by = EXCLUDED.updated_by, last_error = NULL
RETURNING *;

-- tenant-lint: cross-org worker taraması (rls.System); sırası gelen zamanlanmış raporlar
-- name: DueReportSchedules :many
-- Etkin, engagement'ı açık, günü gelmiş ve bu dönemi henüz gönderilmemiş
-- zamanlamalar.
SELECT s.organization_id, s.engagement_id, s.taxpayer_id, s.recipient,
       e.uuid AS engagement_uuid, o.name AS office_name
FROM report_schedules s
JOIN engagements e ON e.organization_id = s.organization_id AND e.id = s.engagement_id
JOIN organizations o ON o.id = s.organization_id
WHERE s.enabled AND e.status = 'active' AND e.ended_at IS NULL
  AND s.day_of_month <= @today::int
  AND (s.last_period IS NULL OR s.last_period < @period::text)
ORDER BY s.organization_id, s.engagement_id
LIMIT @batch_limit
FOR UPDATE OF s SKIP LOCKED;

-- name: MarkReportScheduleRun :exec
UPDATE report_schedules SET last_period = @period, last_error = sqlc.narg(last_error)
WHERE organization_id = @organization_id AND engagement_id = @engagement_id;
