-- Geç gelen belge ve geçmiş dönem takibi (#780 C).

-- name: ScanLateDocuments :execrows
-- Kapanmış dönemlere (kilit ya da verilmiş beyan) kapanıştan sonra gelen
-- belgeleri late_documents'a yazar; aynı belge ikinci kez yazılmaz.
-- engagement_id boşsa ofisin (görünen) bütün mükellefleri taranır.
WITH closes AS (
    SELECT c.organization_id, c.engagement_id, c.period::text AS period,
           min(c.at)::timestamptz AS closed_at,
           (array_agg(c.reason ORDER BY c.at))[1]::text AS reason
    FROM (
        SELECT pl.organization_id, pl.engagement_id, pl.period, pl.locked_at AS at, 'lock'::text AS reason
        FROM period_locks pl
        WHERE pl.organization_id = @organization_id
          AND (sqlc.narg(engagement_id)::bigint IS NULL OR pl.engagement_id = sqlc.narg(engagement_id)::bigint)
        UNION ALL
        SELECT tr.organization_id, tr.engagement_id, tr.period, tr.filed_at AS at, 'filed'::text AS reason
        FROM tax_returns tr
        WHERE tr.organization_id = @organization_id
          AND (sqlc.narg(engagement_id)::bigint IS NULL OR tr.engagement_id = sqlc.narg(engagement_id)::bigint)
          AND tr.status = 'filed' AND tr.filed_at IS NOT NULL
    ) c
    GROUP BY c.organization_id, c.engagement_id, c.period
),
scoped AS (
    SELECT cl.organization_id, cl.engagement_id, cl.period, cl.closed_at, cl.reason, e.taxpayer_id
    FROM closes cl
    JOIN engagements e ON e.organization_id = cl.organization_id AND e.id = cl.engagement_id
),
docs AS (
    SELECT s.organization_id, s.engagement_id, 'invoice'::text AS document_type, i.id AS document_id,
           i.uuid AS document_uuid, s.period, i.created_at AS arrived_at, s.closed_at, s.reason
    FROM scoped s
    JOIN invoices i ON i.taxpayer_id = s.taxpayer_id AND i.period = s.period
    WHERE i.created_at > s.closed_at AND i.status_normalized <> 'cancelled'
    UNION ALL
    SELECT s.organization_id, s.engagement_id, 'expense'::text, x.id, x.uuid, s.period, x.created_at, s.closed_at, s.reason
    FROM scoped s
    JOIN expenses x ON x.taxpayer_id = s.taxpayer_id AND x.period = s.period
    WHERE x.created_at > s.closed_at AND x.approval_status <> 'rejected'
    UNION ALL
    SELECT s.organization_id, s.engagement_id, 'retail_sale'::text, r.id, r.uuid, s.period, r.created_at, s.closed_at, s.reason
    FROM scoped s
    JOIN retail_sales r ON r.taxpayer_id = s.taxpayer_id AND r.period = s.period
    WHERE r.created_at > s.closed_at AND r.approval_status <> 'rejected'
)
INSERT INTO late_documents (organization_id, engagement_id, document_type, document_id, document_uuid,
                            period, arrived_at, closed_at, close_reason)
SELECT d.organization_id, d.engagement_id, d.document_type, d.document_id, d.document_uuid,
       d.period, d.arrived_at, d.closed_at, d.reason
FROM docs d
ON CONFLICT (engagement_id, document_type, document_id) DO NOTHING;

-- name: ListLateDocuments :many
-- Mükellefin geç gelen belgeleri; bekleyenler önce, yeni dönem önce.
SELECT ld.uuid, ld.document_type, ld.document_uuid, ld.period::text AS period, ld.arrived_at, ld.closed_at,
       ld.close_reason, ld.status, COALESCE(ld.target_period::text, '')::text AS target_period, ld.note, ld.decided_at,
       COALESCE(NULLIF(btrim(u.name || ' ' || u.surname), ''), '')::text AS decided_by_name,
       COALESCE(i.invoice_number::text, x.receipt_number, NULLIF(x.description, ''), r.z_report_number, '')::text AS label,
       COALESCE(i.customer_name, x.vendor_name, '')::text AS counterparty,
       COALESCE(to_char(COALESCE(i.issue_date, x.expense_date, r.sale_date), 'YYYY-MM-DD'), '')::text AS document_date,
       COALESCE(i.payable_amount, x.amount, r.total_amount, 0)::numeric(18,2)::text AS amount,
       COALESCE(i.currency::text, x.currency::text, 'TRY')::text AS currency,
       COALESCE(i.direction, '')::text AS invoice_direction,
       COALESCE(i.vat_deduction_period, '')::text AS vat_deduction_period
FROM late_documents ld
JOIN engagements e ON e.organization_id = ld.organization_id AND e.id = ld.engagement_id
LEFT JOIN users u ON u.id = ld.decided_by
LEFT JOIN invoices i ON ld.document_type = 'invoice' AND i.taxpayer_id = e.taxpayer_id AND i.id = ld.document_id
LEFT JOIN expenses x ON ld.document_type = 'expense' AND x.taxpayer_id = e.taxpayer_id AND x.id = ld.document_id
LEFT JOIN retail_sales r ON ld.document_type = 'retail_sale' AND r.taxpayer_id = e.taxpayer_id AND r.id = ld.document_id
WHERE ld.organization_id = @organization_id AND ld.engagement_id = @engagement_id
  AND (sqlc.narg(status)::text IS NULL OR ld.status = sqlc.narg(status)::text)
  AND (sqlc.narg(period)::text IS NULL OR ld.period = sqlc.narg(period)::text)
  AND (sqlc.narg(document_uuid)::uuid IS NULL OR ld.document_uuid = sqlc.narg(document_uuid)::uuid)
ORDER BY (ld.status = 'pending') DESC, ld.period DESC, ld.arrived_at DESC, ld.id DESC
LIMIT 500;

-- name: CountLateDocumentsByStatus :many
SELECT status, count(*)::bigint AS n
FROM late_documents
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
GROUP BY status
ORDER BY status;

-- name: GetLateDocument :one
SELECT * FROM late_documents
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid;

-- name: DecideLateDocument :one
-- Ofisin kararı; "pending" kararı geri alır.
UPDATE late_documents
SET status = @status,
    target_period = sqlc.narg(target_period),
    note = sqlc.narg(note),
    decided_by = CASE WHEN @status::text = 'pending' THEN NULL ELSE sqlc.narg(decided_by)::bigint END,
    decided_at = CASE WHEN @status::text = 'pending' THEN NULL ELSE NOW() END,
    updated_at = NOW()
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND uuid = @uuid
RETURNING *;

-- tenant-lint: org-wide ofis geneli geçmiş dönem takibi sayacı; read_all yoksa atananlarla sınırlı
-- name: SummarizeLateDocuments :many
SELECT e.uuid AS engagement_uuid, tp.title AS taxpayer_title,
       count(*) FILTER (WHERE ld.status = 'pending')::bigint AS pending,
       count(*)::bigint AS total,
       COALESCE(min(ld.period) FILTER (WHERE ld.status = 'pending'), '')::text AS oldest_pending_period
FROM late_documents ld
JOIN engagements e ON e.organization_id = ld.organization_id AND e.id = ld.engagement_id
JOIN taxpayers tp ON tp.id = e.taxpayer_id
WHERE ld.organization_id = @organization_id AND e.ended_at IS NULL
  AND (@all_engagements::bool OR EXISTS (
      SELECT 1 FROM engagement_assignments a
      WHERE a.organization_id = ld.organization_id AND a.engagement_id = ld.engagement_id AND a.user_id = @user_id))
GROUP BY e.uuid, tp.title
ORDER BY 3 DESC, tp.title
LIMIT 500;
