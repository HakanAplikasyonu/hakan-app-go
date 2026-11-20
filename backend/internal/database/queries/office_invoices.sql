-- Ofis paneli fatura görünümü (issue #142). Bir engagement'ın mükellefine ait
-- faturalar; ofis yalnızca kendi dönemini ve mükellefin paylaştığı geçmişi
-- görür (engagement_document_visible, issue #8).

-- name: ListEngagementInvoices :many
-- Dönem süzgecinde KDV'si o aya alınan alış faturası da gelir (#640): ofis
-- geç gelen faturayı beyan ettiği ayda görür.
-- Sayfa önce yalnızca kimliklerle seçilir; açık uyarı sayısı yalnızca dönen
-- satırlar için hesaplanır (OFFSET'te atlanan satırlar için değil, #182).
WITH page AS (
    SELECT i.id FROM invoices i
    WHERE i.taxpayer_id = @taxpayer_id
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
      AND (sqlc.narg(period)::text IS NULL OR i.period = sqlc.narg(period) OR i.vat_deduction_period = sqlc.narg(period))
      AND (sqlc.narg(direction)::text IS NULL OR i.direction = sqlc.narg(direction))
    ORDER BY i.issue_date DESC, i.id DESC
    LIMIT @lim OFFSET @off
)
SELECT sqlc.embed(invoices),
       (SELECT COUNT(*) FROM data_warnings w
         WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
           AND w.entity_type = 'invoice' AND w.entity_id = invoices.id AND w.status = 'open')::bigint AS open_warnings
FROM invoices JOIN page ON page.id = invoices.id
ORDER BY invoices.issue_date DESC, invoices.id DESC;

-- name: ListEngagementInvoicesAfter :many
-- İmleçli sayfa (#182): (issue_date, id) imlecinden sonraki satırlar. Satır
-- karşılaştırması idx_invoices_taxpayer_issue indeksinde sınır koşuludur,
-- derin sayfa da ilk sayfa kadar ucuzdur.
WITH page AS (
    SELECT i.id FROM invoices i
    WHERE i.taxpayer_id = @taxpayer_id
      AND (i.issue_date, i.id) < (@cursor_date::date, @cursor_id::bigint)
      AND engagement_document_visible(@engagement_id::bigint, i.issue_date)
      AND (sqlc.narg(period)::text IS NULL OR i.period = sqlc.narg(period) OR i.vat_deduction_period = sqlc.narg(period))
      AND (sqlc.narg(direction)::text IS NULL OR i.direction = sqlc.narg(direction))
    ORDER BY i.issue_date DESC, i.id DESC
    LIMIT @lim
)
SELECT sqlc.embed(invoices),
       (SELECT COUNT(*) FROM data_warnings w
         WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
           AND w.entity_type = 'invoice' AND w.entity_id = invoices.id AND w.status = 'open')::bigint AS open_warnings
FROM invoices JOIN page ON page.id = invoices.id
ORDER BY invoices.issue_date DESC, invoices.id DESC;

-- name: CountEngagementInvoices :one
-- Görünürlük yalnızca tarihe bağlı olduğundan her farklı tarih için bir kez
-- sorulur, satır başına değil (#182). MATERIALIZED, planlayıcının koşulu
-- yeniden satır taramasına itmesini engeller.
WITH d AS MATERIALIZED (
    SELECT issue_date, COUNT(*) AS n FROM invoices
    WHERE taxpayer_id = @taxpayer_id
      AND (sqlc.narg(period)::text IS NULL OR period = sqlc.narg(period) OR vat_deduction_period = sqlc.narg(period))
      AND (sqlc.narg(direction)::text IS NULL OR direction = sqlc.narg(direction))
    GROUP BY issue_date
)
SELECT COALESCE(SUM(d.n), 0)::bigint FROM d
WHERE engagement_document_visible(@engagement_id::bigint, d.issue_date);

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
