-- Ofis paneli fatura görünümü (issue #142). Bir engagement'ın mükellefine ait
-- faturalar; ofis yalnızca kendi dönemini ve mükellefin paylaştığı geçmişi
-- görür (engagement_document_visible, issue #8).

-- name: ListEngagementInvoices :many
SELECT sqlc.embed(invoices),
       (SELECT COUNT(*) FROM data_warnings w
         WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
           AND w.entity_type = 'invoice' AND w.entity_id = invoices.id AND w.status = 'open')::bigint AS open_warnings
FROM invoices
WHERE invoices.taxpayer_id = @taxpayer_id
  AND engagement_document_visible(@engagement_id::bigint, invoices.issue_date)
  AND (sqlc.narg(period)::text IS NULL OR invoices.period = sqlc.narg(period))
  AND (sqlc.narg(direction)::text IS NULL OR invoices.direction = sqlc.narg(direction))
ORDER BY invoices.issue_date DESC, invoices.id DESC
LIMIT @lim OFFSET @off;

-- name: CountEngagementInvoices :one
SELECT COUNT(*)::bigint FROM invoices
WHERE taxpayer_id = @taxpayer_id
  AND engagement_document_visible(@engagement_id::bigint, issue_date)
  AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period))
  AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction));

-- name: GetEngagementInvoice :one
SELECT * FROM invoices
WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid
  AND engagement_document_visible(@engagement_id::bigint, issue_date);

-- name: ListInvoiceWarnings :many
-- Faturanın ve satırlarının bu engagement'taki uyarıları.
SELECT w.* FROM data_warnings w
WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
  AND ((w.entity_type = 'invoice' AND w.entity_id = @invoice_id)
    OR (w.entity_type = 'invoice_item' AND w.entity_id IN (
        SELECT ii.id FROM invoice_items ii WHERE ii.taxpayer_id = @taxpayer_id AND ii.invoice_id = @invoice_id)))
ORDER BY w.created_at, w.id;

-- name: GetInvoiceAttachment :one
SELECT * FROM attachments
WHERE taxpayer_id = @taxpayer_id AND id = @id;
