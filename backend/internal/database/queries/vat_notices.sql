-- Mükellefe giden KDV bildirimi (#780): ne söylendi, ne zaman.

-- name: InsertVATNotice :one
INSERT INTO vat_notices (organization_id, engagement_id, taxpayer_id, period, outcome,
    output_vat, input_vat, carried_in, payable, carry_out, due_date, message, sent_by)
VALUES (@organization_id, @engagement_id, @taxpayer_id, @period, @outcome,
    @output_vat, @input_vat, @carried_in, @payable, @carry_out, sqlc.narg(due_date), sqlc.narg(message), sqlc.narg(sent_by))
RETURNING *;

-- name: SetVATNoticeNotifications :exec
UPDATE vat_notices SET notification_uuids = @notification_uuids::uuid[]
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND id = @id;

-- name: ListVATNotices :many
-- Bir mükellefin bildirimleri, yeniden eskiye; gönderen adıyla.
SELECT n.*, COALESCE(TRIM(u.name || ' ' || u.surname), '')::text AS sent_by_name
FROM vat_notices n
LEFT JOIN users u ON u.id = n.sent_by
WHERE n.organization_id = @organization_id AND n.engagement_id = @engagement_id
ORDER BY n.created_at DESC, n.id DESC
LIMIT @limit_count;

-- name: ListVATNoticeRecipients :many
-- Bildirimi alacak mükellef kullanıcıları.
SELECT user_id FROM taxpayer_members WHERE taxpayer_id = @taxpayer_id ORDER BY user_id;
