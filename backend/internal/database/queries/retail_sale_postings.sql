-- Z raporu yönetimi (issue #384): KDV / ödeme kırılımı, ÖKC bazında liste,
-- süreklilik denetimi, onay ve satış fişi.

-- name: SetRetailSaleBreakdown :one
-- Kırılım ve başlık düzeltmesi (yalnızca bekleyen kayıt). NULL verilen alan
-- değişmez; subtotal toplam - KDV ile yeniden hesaplanır.
UPDATE retail_sales SET
    sale_date            = COALESCE(sqlc.narg(sale_date)::date, sale_date),
    period               = COALESCE(to_char(sqlc.narg(sale_date)::date, 'YYYY-MM'), period),
    z_report_number      = COALESCE(sqlc.narg(z_report_number)::text, z_report_number),
    okc_serial_number    = COALESCE(sqlc.narg(okc_serial_number)::text, okc_serial_number),
    total_amount         = COALESCE(sqlc.narg(total_amount)::numeric, total_amount),
    tax_amount           = COALESCE(sqlc.narg(tax_amount)::numeric, tax_amount),
    subtotal_amount      = COALESCE(sqlc.narg(total_amount)::numeric, total_amount) - COALESCE(sqlc.narg(tax_amount)::numeric, tax_amount, 0),
    cash_amount          = COALESCE(sqlc.narg(cash_amount)::numeric, cash_amount),
    card_amount          = COALESCE(sqlc.narg(card_amount)::numeric, card_amount),
    other_payment_amount = COALESCE(sqlc.narg(other_payment_amount)::numeric, other_payment_amount),
    receipt_count        = COALESCE(sqlc.narg(receipt_count)::int, receipt_count)
WHERE taxpayer_id = @taxpayer_id AND id = @id AND approval_status = 'pending'
RETURNING *;

-- name: DeleteRetailSaleVatLines :exec
DELETE FROM retail_sale_vat_lines WHERE taxpayer_id = @taxpayer_id AND retail_sale_id = @retail_sale_id;

-- name: InsertRetailSaleVatLines :execrows
-- rows: [{"tax_rate","tax_base","tax_amount"}, ...]
INSERT INTO retail_sale_vat_lines (taxpayer_id, retail_sale_id, tax_rate, tax_base, tax_amount)
SELECT @taxpayer_id, @retail_sale_id, r.tax_rate, r.tax_base, r.tax_amount
FROM jsonb_to_recordset(@rows::jsonb) AS r(tax_rate numeric, tax_base numeric, tax_amount numeric);

-- name: ListRetailSaleVatLines :many
SELECT retail_sale_id, tax_rate, tax_base, tax_amount FROM retail_sale_vat_lines
WHERE taxpayer_id = @taxpayer_id AND retail_sale_id = ANY(@ids::bigint[])
ORDER BY retail_sale_id, tax_rate;

-- name: ListEngagementRetailSales :many
SELECT s.id, s.uuid, s.source_system, s.capture_method, s.sale_date, s.period, s.z_report_number, s.okc_serial_number,
       s.subtotal_amount, s.tax_amount, s.total_amount, s.currency, s.cash_amount, s.card_amount,
       s.other_payment_amount, s.receipt_count, s.approval_status, s.ocr_confidence, s.attachment_id,
       s.approved_at, s.created_at,
       j.uuid AS journal_uuid, j.entry_no AS journal_entry_no, j.status AS journal_status,
       COALESCE((SELECT array_agg(DISTINCT w.code ORDER BY w.code) FROM data_warnings w
                 WHERE w.organization_id = @organization_id AND w.engagement_id = @engagement_id
                   AND w.entity_type = 'retail_sale' AND w.entity_id = s.id AND w.status = 'open'), '{}')::text[] AS warning_codes
FROM retail_sales s
LEFT JOIN receipt_postings p ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id
     AND p.target_type = 'retail_sale' AND p.target_id = s.id
LEFT JOIN journal_drafts j ON j.organization_id = @organization_id AND j.engagement_id = @engagement_id
     AND j.id = p.journal_draft_id
WHERE s.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR s.period = sqlc.narg(period)::text)
  AND (sqlc.narg(status)::text IS NULL OR s.approval_status = sqlc.narg(status)::text)
  AND (sqlc.narg(okc)::text IS NULL OR s.okc_serial_number = sqlc.narg(okc)::text
       OR (sqlc.narg(okc)::text = '-' AND s.okc_serial_number IS NULL))
ORDER BY s.okc_serial_number NULLS LAST, s.sale_date DESC NULLS LAST, s.id DESC
LIMIT @lim OFFSET @off;

-- name: SummarizeEngagementRetailSales :one
SELECT count(*)::bigint AS total,
       count(*) FILTER (WHERE s.approval_status = 'pending')::bigint AS pending,
       COALESCE(sum(s.total_amount), 0)::numeric AS total_amount,
       COALESCE(sum(s.tax_amount), 0)::numeric AS tax_amount,
       COALESCE(sum(s.cash_amount), 0)::numeric AS cash_amount,
       COALESCE(sum(s.card_amount), 0)::numeric AS card_amount
FROM retail_sales s
WHERE s.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR s.period = sqlc.narg(period)::text)
  AND (sqlc.narg(status)::text IS NULL OR s.approval_status = sqlc.narg(status)::text)
  AND (sqlc.narg(okc)::text IS NULL OR s.okc_serial_number = sqlc.narg(okc)::text
       OR (sqlc.narg(okc)::text = '-' AND s.okc_serial_number IS NULL));

-- name: SummarizeRetailSaleVatRates :many
-- Filtredeki Z raporlarının KDV oranı kırılımı.
SELECT v.tax_rate, COALESCE(sum(v.tax_base), 0)::numeric AS tax_base, COALESCE(sum(v.tax_amount), 0)::numeric AS tax_amount
FROM retail_sale_vat_lines v
JOIN retail_sales s ON s.taxpayer_id = v.taxpayer_id AND s.id = v.retail_sale_id
WHERE v.taxpayer_id = @taxpayer_id
  AND (sqlc.narg(period)::text IS NULL OR s.period = sqlc.narg(period)::text)
  AND (sqlc.narg(status)::text IS NULL OR s.approval_status = sqlc.narg(status)::text)
  AND (sqlc.narg(okc)::text IS NULL OR s.okc_serial_number = sqlc.narg(okc)::text
       OR (sqlc.narg(okc)::text = '-' AND s.okc_serial_number IS NULL))
GROUP BY v.tax_rate
ORDER BY v.tax_rate;

-- name: ListRetailSaleDevices :many
-- Mükellefin ÖKC seri numaraları (Z sayısı, son Z tarihi).
SELECT COALESCE(okc_serial_number, '')::text AS okc_serial_number, count(*)::bigint AS reports,
       max(sale_date)::date AS last_date
FROM retail_sales
WHERE taxpayer_id = @taxpayer_id AND approval_status <> 'rejected'
GROUP BY okc_serial_number
ORDER BY okc_serial_number NULLS LAST;

-- name: ListRetailSalesForContinuity :many
SELECT id, sale_date, z_report_number, period FROM retail_sales
WHERE taxpayer_id = @taxpayer_id AND okc_serial_number = @okc_serial_number
  AND approval_status <> 'rejected' AND sale_date IS NOT NULL
ORDER BY sale_date, id;

-- name: GetRetailSaleByUUID :one
SELECT * FROM retail_sales WHERE taxpayer_id = @taxpayer_id AND uuid = @uuid;

-- name: SetCaptureStatusForRetailSale :exec
UPDATE receipt_captures SET status = @status
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND taxpayer_id = @taxpayer_id AND retail_sale_id = @retail_sale_id AND status = 'draft';

-- name: OpenWarningCodesForEntity :many
SELECT DISTINCT code FROM data_warnings
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND entity_type = @entity_type AND entity_id = @entity_id AND status = 'open'
ORDER BY code;
