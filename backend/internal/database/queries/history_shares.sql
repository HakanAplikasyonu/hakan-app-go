-- Ofis geçişi: geçmiş veri paylaşımı ve engagement'ın gördüğü belgeler
-- (issue #8, D2).

-- name: ListHistorySharesForTaxpayer :many
SELECT s.uuid, s.from_date, s.to_date, s.granted_at, s.revoked_at,
       o.name AS organization_name, (e.ended_at IS NULL)::boolean AS engagement_active
FROM taxpayer_history_shares s
INNER JOIN engagements e ON e.id = s.engagement_id AND e.organization_id = s.organization_id
INNER JOIN organizations o ON o.id = s.organization_id
WHERE s.taxpayer_id = sqlc.arg(taxpayer_id)
ORDER BY s.granted_at DESC, s.id DESC
LIMIT 100;

-- tenant-lint: cross-org mükellefin kendi aktif engagement'ı (hangi ofiste olursa)
-- name: GetActiveEngagementOfTaxpayer :one
SELECT e.id, e.organization_id, e.started_at
FROM engagements e
WHERE e.taxpayer_id = sqlc.arg(taxpayer_id) AND e.ended_at IS NULL;

-- name: CreateHistoryShare :one
INSERT INTO taxpayer_history_shares (taxpayer_id, organization_id, engagement_id, from_date, to_date, granted_by)
VALUES (sqlc.arg(taxpayer_id), sqlc.arg(organization_id), sqlc.arg(engagement_id),
        sqlc.narg(from_date), sqlc.narg(to_date), sqlc.narg(granted_by))
RETURNING uuid, granted_at;

-- name: RevokeHistoryShare :execrows
UPDATE taxpayer_history_shares
SET revoked_at = NOW(), revoked_by = sqlc.narg(revoked_by)
WHERE taxpayer_id = sqlc.arg(taxpayer_id) AND uuid = sqlc.arg(uuid) AND revoked_at IS NULL;

-- name: ListHistorySharesForEngagement :many
SELECT s.uuid, s.from_date, s.to_date, s.granted_at, s.revoked_at
FROM taxpayer_history_shares s
WHERE s.taxpayer_id = sqlc.arg(taxpayer_id)
  AND s.organization_id = sqlc.arg(organization_id)
  AND s.engagement_id = sqlc.arg(engagement_id)
ORDER BY s.granted_at DESC, s.id DESC;

-- Belgeler (faturalar) bir engagement'ın gördüğü kadarıyla. Görünürlük
-- kuralı engagement_document_visible'da (migration 000048).
-- name: ListEngagementVisibleInvoices :many
SELECT i.uuid, i.invoice_number, i.issue_date, i.direction, i.payable_amount, i.currency
FROM invoices i
WHERE i.taxpayer_id = sqlc.arg(taxpayer_id)
  AND engagement_document_visible(sqlc.arg(engagement_id)::bigint, i.issue_date)
ORDER BY i.issue_date DESC, i.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);
