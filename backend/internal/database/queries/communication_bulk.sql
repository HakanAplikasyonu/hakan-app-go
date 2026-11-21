-- İletişim merkezi (#780 B): mükellef grupları ve elle toplu gönderim.

-- name: CreateTaxpayerGroup :one
INSERT INTO taxpayer_groups (organization_id, name, description, created_by)
VALUES (@organization_id, @name, @description, sqlc.narg(created_by))
RETURNING *;

-- name: UpdateTaxpayerGroup :one
UPDATE taxpayer_groups
SET name = @name, description = @description, updated_at = NOW()
WHERE organization_id = @organization_id AND uuid = @uuid
RETURNING *;

-- name: DeleteTaxpayerGroup :execrows
DELETE FROM taxpayer_groups WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: GetTaxpayerGroup :one
SELECT * FROM taxpayer_groups WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: ListTaxpayerGroups :many
SELECT g.*,
    (SELECT count(*) FROM taxpayer_group_members m
     JOIN engagements e ON e.organization_id = m.organization_id AND e.id = m.engagement_id AND e.status = 'active'
     WHERE m.organization_id = g.organization_id AND m.group_id = g.id)::int AS member_count
FROM taxpayer_groups g
WHERE g.organization_id = @organization_id
ORDER BY lower(g.name), g.id;

-- name: ClearTaxpayerGroupMembers :exec
DELETE FROM taxpayer_group_members WHERE organization_id = @organization_id AND group_id = @group_id;

-- name: AddTaxpayerGroupMembers :execrows
INSERT INTO taxpayer_group_members (organization_id, group_id, engagement_id)
SELECT e.organization_id, @group_id, e.id
FROM engagements e
WHERE e.organization_id = @organization_id AND e.status = 'active' AND e.uuid = ANY(@engagement_uuids::uuid[])
ON CONFLICT DO NOTHING;

-- name: ListTaxpayerGroupMembers :many
SELECT e.uuid AS engagement_uuid, t.title, COALESCE(t.tax_id, '')::text AS tax_id
FROM taxpayer_group_members m
JOIN engagements e ON e.organization_id = m.organization_id AND e.id = m.engagement_id
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE m.organization_id = @organization_id AND m.group_id = @group_id AND e.status = 'active'
ORDER BY t.title, e.id;

-- name: GroupEngagementIDs :many
SELECT m.engagement_id FROM taxpayer_group_members m
WHERE m.organization_id = @organization_id AND m.group_id = @group_id
ORDER BY m.engagement_id;

-- name: EngagementIDsByUUIDs :many
SELECT e.id FROM engagements e
WHERE e.organization_id = @organization_id AND e.status = 'active' AND e.uuid = ANY(@engagement_uuids::uuid[])
ORDER BY e.id;

-- Dinamik süzgeç: beyan türü (son dönemlerde o türden beyan kaydı olan) ve
-- sorumlu çalışan (atanmış üye). Boş süzgeç koşul koymaz.
-- name: EngagementIDsByFilter :many
SELECT e.id FROM engagements e
WHERE e.organization_id = @organization_id AND e.status = 'active'
  AND (@tax_kind::text = '' OR EXISTS (
        SELECT 1 FROM tax_returns r
        WHERE r.organization_id = e.organization_id AND r.engagement_id = e.id
          AND r.kind = @tax_kind::text AND r.period >= @since_period::text))
  AND (sqlc.narg(assignee)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM engagement_assignments a JOIN users u ON u.id = a.user_id
        WHERE a.organization_id = e.organization_id AND a.engagement_id = e.id AND u.uuid = sqlc.narg(assignee)::uuid))
ORDER BY e.id;

-- name: ListMessageContacts :many
SELECT e.id AS engagement_id, e.uuid AS engagement_uuid, t.title,
    COALESCE(t.contact_phone, '')::text AS contact_phone,
    COALESCE(t.contact_email::text, '')::text AS contact_email
FROM engagements e
JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE e.organization_id = @organization_id AND e.status = 'active' AND e.id = ANY(@engagement_ids::bigint[])
ORDER BY t.title, e.id;

-- name: CountEngagementsByTaxKind :many
SELECT r.kind, count(DISTINCT r.engagement_id)::int AS engagements
FROM tax_returns r
JOIN engagements e ON e.organization_id = r.organization_id AND e.id = r.engagement_id AND e.status = 'active'
WHERE r.organization_id = @organization_id AND r.period >= @since_period::text
GROUP BY r.kind
ORDER BY r.kind;

-- name: CountEngagementsByAssignee :many
SELECT u.uuid AS user_uuid, u.name, u.surname, u.email::text AS email, count(*)::int AS engagements
FROM engagement_assignments a
JOIN engagements e ON e.organization_id = a.organization_id AND e.id = a.engagement_id AND e.status = 'active'
JOIN users u ON u.id = a.user_id
WHERE a.organization_id = @organization_id
GROUP BY u.uuid, u.name, u.surname, u.email
ORDER BY u.name, u.surname;

-- name: GetOrganizationNameForMessage :one
SELECT name::text FROM organizations WHERE id = @organization_id;

-- name: InsertBulkSend :one
INSERT INTO bulk_sends (organization_id, channel, subject, body, target_kind, target_label, engagement_ids,
    total, duplicates, skipped, created_by)
VALUES (@organization_id, @channel, @subject, @body, @target_kind, @target_label, @engagement_ids::bigint[],
    @total, @duplicates, @skipped::jsonb, sqlc.narg(created_by))
RETURNING *;

-- name: GetBulkSend :one
SELECT * FROM bulk_sends WHERE organization_id = @organization_id AND uuid = @uuid;

-- name: StepBulkSend :exec
UPDATE bulk_sends
SET processed = processed + 1, sent = sent + @sent_delta::int, failed = failed + @failed_delta::int,
    failures = CASE WHEN sqlc.narg(failure)::jsonb IS NULL THEN failures ELSE failures || jsonb_build_array(sqlc.narg(failure)::jsonb) END,
    updated_at = NOW()
WHERE organization_id = @organization_id AND id = @id;

-- name: FinishBulkSend :exec
UPDATE bulk_sends
SET status = @status, error_message = @error_message, updated_at = NOW(), finished_at = NOW()
WHERE organization_id = @organization_id AND id = @id;

-- name: ListRecentBulkSends :many
SELECT * FROM bulk_sends
WHERE organization_id = @organization_id AND created_by = @created_by
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- Başarısız (ve sonucu belirsiz) gönderimler: toplu gönderimin sonunda listelenir.
-- name: ListBulkSendFailures :many
SELECT COALESCE(t.title, '')::text AS title, m.phone AS recipient, m.status, m.error_message
FROM sms_messages m
LEFT JOIN engagements e ON e.organization_id = m.organization_id AND e.id = m.engagement_id
LEFT JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE m.organization_id = @organization_id AND m.bulk_id = @bulk_id AND m.status IN ('failed', 'uncertain')
UNION ALL
SELECT COALESCE(t.title, '')::text AS title, m.to_email::text AS recipient, m.status, m.error_message
FROM mail_messages m
LEFT JOIN engagements e ON e.organization_id = m.organization_id AND e.id = m.engagement_id
LEFT JOIN taxpayers t ON t.id = e.taxpayer_id
WHERE m.organization_id = @organization_id AND m.bulk_id = @bulk_id AND m.status = 'failed'
ORDER BY 1;
