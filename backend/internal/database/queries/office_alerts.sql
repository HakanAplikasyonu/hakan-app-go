-- Kritik uyarı bildirimi (issue #241). Zamanlanmış iş bütün ofisleri tarar.

-- tenant-lint: cross-org zamanlanmış iş bütün ofislerin bildirilmemiş açık engelleyicilerini tarar
-- name: ListUnalertedBlockerWarnings :many
SELECT w.organization_id, w.engagement_id, w.taxpayer_id,
       count(*)::bigint AS warning_count,
       array_agg(w.id ORDER BY w.id)::bigint[] AS warning_ids
FROM data_warnings w
JOIN engagements e ON e.organization_id = w.organization_id AND e.id = w.engagement_id AND e.ended_at IS NULL
WHERE w.status = 'open' AND w.severity = 'blocker' AND w.created_at >= @since
  AND NOT EXISTS (SELECT 1 FROM warning_office_alerts a WHERE a.warning_id = w.id)
GROUP BY w.organization_id, w.engagement_id, w.taxpayer_id
ORDER BY w.organization_id, w.engagement_id
LIMIT @lim;

-- name: ListEngagementAlertRecipients :many
-- Atanan ofis üyeleri (hâlâ üye olanlar); atanan yoksa ofis sahipleri.
WITH assigned AS (
    SELECT a.user_id FROM engagement_assignments a
    JOIN organization_members m ON m.organization_id = a.organization_id AND m.user_id = a.user_id
    WHERE a.organization_id = @organization_id AND a.engagement_id = @engagement_id
)
SELECT user_id FROM assigned
UNION
SELECT m.user_id FROM organization_members m
JOIN roles r ON r.id = m.role_id
WHERE m.organization_id = @organization_id AND r.slug = 'owner' AND NOT EXISTS (SELECT 1 FROM assigned)
ORDER BY 1;

-- name: GetEngagementAlertNames :one
SELECT o.slug AS organization_slug, e.uuid AS engagement_uuid, tp.title AS taxpayer_title
FROM engagements e
JOIN organizations o ON o.id = e.organization_id
JOIN taxpayers tp ON tp.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.id = @engagement_id;

-- name: InsertWarningOfficeAlerts :execrows
INSERT INTO warning_office_alerts (warning_id, organization_id, engagement_id, recipients)
SELECT unnest(@warning_ids::bigint[]), @organization_id, @engagement_id, @recipients
ON CONFLICT (warning_id) DO NOTHING;
