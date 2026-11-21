-- Defter türü ve işletme hesabı (#780). İşletme defteri ve serbest meslek
-- kazanç defteri tutan mükellefte hesap planı yoktur: alış faturası, masraf
-- ve fiş gider; satış faturası ve Z raporu gelir satırıdır. Fatura kuralları
-- KDV1 ile aynı (tax_returns.sql): aktif, kaynakta silinmemiş, dövizliyse
-- kuru olan faturalar; masraf ve Z raporu onaylı ve TL.

-- name: GetEngagementLedgerType :one
SELECT ledger_type FROM engagements
WHERE organization_id = @organization_id AND id = @engagement_id;

-- name: SetEngagementLedgerType :one
UPDATE engagements SET ledger_type = @ledger_type
WHERE organization_id = @organization_id AND id = @engagement_id
RETURNING ledger_type;

-- name: ListOperatingAccountRows :many
SELECT x.source, x.record_uuid, x.doc_date, x.period, x.document_no, x.counterparty,
       x.direction, x.doc_type, x.doc_class, x.category_name, x.net, x.vat,
       COALESCE(o.category, '')::text AS override_category
FROM (
    SELECT 'invoice'::text AS source, i.uuid AS record_uuid, i.issue_date AS doc_date, i.period::text AS period,
           i.invoice_number::text AS document_no, COALESCE(i.customer_name, '')::text AS counterparty,
           i.direction::text AS direction, i.type::text AS doc_type, i.document_class::text AS doc_class,
           ''::text AS category_name,
           ROUND(COALESCE(i.tax_exclusive_amount, 0) * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2)::numeric AS net,
           ROUND(COALESCE(i.tax_amount, 0) * (CASE WHEN i.currency = 'TRY' THEN 1 ELSE i.exchange_rate END), 2)::numeric AS vat
    FROM invoices i
    WHERE i.taxpayer_id = @taxpayer_id
      AND i.period >= @from_period::text AND i.period <= @to_period::text
      AND i.status_normalized = 'active'
      AND i.deleted_at_source IS NULL
      AND (i.currency = 'TRY' OR i.exchange_rate IS NOT NULL)
    UNION ALL
    SELECT 'expense'::text, e.uuid, e.expense_date, COALESCE(e.period, '')::text,
           COALESCE(e.receipt_number, '')::text, COALESCE(e.vendor_name, '')::text,
           'incoming'::text, e.receipt_type::text, ''::text,
           COALESCE((SELECT c.name FROM expense_categories c
                     WHERE c.organization_id = @organization_id
                       AND (c.engagement_id IS NULL OR c.engagement_id = @engagement_id::bigint)
                       AND c.code = p.category_code
                     ORDER BY c.engagement_id NULLS LAST LIMIT 1), '')::text,
           ROUND(COALESCE(e.tax_exclusive_amount, e.amount - COALESCE(e.tax_amount, 0), 0), 2)::numeric,
           ROUND(COALESCE(e.tax_amount, 0), 2)::numeric
    FROM expenses e
    LEFT JOIN receipt_postings p ON p.organization_id = @organization_id AND p.engagement_id = @engagement_id
         AND p.target_type = 'expense' AND p.target_id = e.id
    WHERE e.taxpayer_id = @taxpayer_id
      AND e.period >= @from_period::text AND e.period <= @to_period::text
      AND e.approval_status = 'approved'
      AND e.currency = 'TRY'
    UNION ALL
    SELECT 'retail_sale'::text, r.uuid, r.sale_date, COALESCE(r.period, '')::text,
           COALESCE(r.z_report_number, '')::text, ''::text,
           'outgoing'::text, 'Z'::text, ''::text, ''::text,
           ROUND(COALESCE(r.subtotal_amount, r.total_amount - COALESCE(r.tax_amount, 0), 0), 2)::numeric,
           ROUND(COALESCE(r.tax_amount, 0), 2)::numeric
    FROM retail_sales r
    WHERE r.taxpayer_id = @taxpayer_id
      AND r.period >= @from_period::text AND r.period <= @to_period::text
      AND r.approval_status = 'approved'
      AND r.invoice_id IS NULL
      AND r.currency = 'TRY'
) x
LEFT JOIN operating_account_categories o ON o.organization_id = @organization_id AND o.engagement_id = @engagement_id
     AND o.source = x.source AND o.record_uuid = x.record_uuid
ORDER BY x.doc_date NULLS LAST, x.source, x.document_no;

-- name: OperatingAccountPending :one
-- Deftere henüz girmeyen kayıtlar: onay bekleyen masraf ve Z raporu, kuru
-- eksik döviz faturası.
SELECT
    (SELECT COUNT(*) FROM expenses e
     WHERE e.taxpayer_id = @taxpayer_id AND e.period >= @from_period::text AND e.period <= @to_period::text
       AND e.approval_status = 'pending')::bigint AS pending_expenses,
    (SELECT COUNT(*) FROM retail_sales r
     WHERE r.taxpayer_id = @taxpayer_id AND r.period >= @from_period::text AND r.period <= @to_period::text
       AND r.approval_status = 'pending')::bigint AS pending_retail_sales,
    (SELECT COUNT(*) FROM invoices i
     WHERE i.taxpayer_id = @taxpayer_id AND i.period >= @from_period::text AND i.period <= @to_period::text
       AND i.status_normalized = 'active' AND i.deleted_at_source IS NULL
       AND i.currency <> 'TRY' AND i.exchange_rate IS NULL)::bigint AS missing_exchange_rate;

-- name: OperatingAccountRecordExists :one
-- Kategori yalnız mükellefin kendi belgesine yazılır.
SELECT EXISTS (
    SELECT 1 FROM invoices i WHERE @source::text = 'invoice' AND i.taxpayer_id = @taxpayer_id AND i.uuid = @record_uuid
    UNION ALL
    SELECT 1 FROM expenses e WHERE @source::text = 'expense' AND e.taxpayer_id = @taxpayer_id AND e.uuid = @record_uuid
    UNION ALL
    SELECT 1 FROM retail_sales r WHERE @source::text = 'retail_sale' AND r.taxpayer_id = @taxpayer_id AND r.uuid = @record_uuid
)::bool;

-- name: UpsertOperatingAccountCategory :exec
INSERT INTO operating_account_categories (organization_id, engagement_id, source, record_uuid, category, decided_by)
VALUES (@organization_id, @engagement_id, @source, @record_uuid, @category, sqlc.narg(decided_by))
ON CONFLICT (organization_id, engagement_id, source, record_uuid) DO UPDATE
SET category = EXCLUDED.category, decided_by = EXCLUDED.decided_by;

-- name: DeleteOperatingAccountCategory :exec
DELETE FROM operating_account_categories
WHERE organization_id = @organization_id AND engagement_id = @engagement_id
  AND source = @source AND record_uuid = @record_uuid;

-- name: GetOperatingAccountInventory :one
SELECT * FROM operating_account_inventories
WHERE organization_id = @organization_id AND engagement_id = @engagement_id AND year = @year;

-- name: UpsertOperatingAccountInventory :one
INSERT INTO operating_account_inventories (organization_id, engagement_id, year, opening_inventory, closing_inventory, updated_by)
VALUES (@organization_id, @engagement_id, @year, @opening_inventory, sqlc.narg(closing_inventory), sqlc.narg(updated_by))
ON CONFLICT (organization_id, engagement_id, year) DO UPDATE
SET opening_inventory = EXCLUDED.opening_inventory, closing_inventory = EXCLUDED.closing_inventory,
    updated_by = EXCLUDED.updated_by
RETURNING *;
