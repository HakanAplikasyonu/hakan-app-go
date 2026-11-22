-- Gelen kutusu (#780 b): müşavirin bu dönem elini bekleyen her şey tek
-- listede. AI önerisi olmayan ve fişi kesilmemiş faturalar, eşleşmemiş banka
-- hareketleri, onay bekleyen masraf fişleri ve Z raporları, mükelleften
-- gelen ve işlenmemiş yabancı hizmet faturaları, karar bekleyen geç
-- belgeler. Her tür kendi içinde @cap ile sınırlıdır; birleşik liste
-- @lim / @off ile sayfalanır. Görünürlük: read_all yoksa yalnız atanan
-- mükellefler; belge görünürlüğü ofis değişiminde engagement_document_visible.

-- tenant-lint: org-wide ofisin gelen kutusu; read_all yoksa yalnız atanan işler (engagement_assignments)
-- name: ListInboxItems :many
WITH eng AS (
    SELECT e.id, e.uuid, e.organization_id, e.taxpayer_id, tp.title
    FROM engagements e
    JOIN taxpayers tp ON tp.id = e.taxpayer_id
    WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
      AND (sqlc.narg(engagement_uuid)::uuid IS NULL OR e.uuid = sqlc.narg(engagement_uuid)::uuid)
      AND (@all_engagements::bool OR EXISTS (
          SELECT 1 FROM engagement_assignments a
          WHERE a.organization_id = e.organization_id AND a.engagement_id = e.id AND a.user_id = @user_id))
),
inv AS (
    SELECT 'invoice'::text AS item_type, i.id AS item_id, i.uuid AS item_uuid,
           eng.uuid AS engagement_uuid, eng.title AS taxpayer_title,
           i.payable_amount::numeric(18,2)::text AS amount, i.currency::text AS currency,
           to_char(i.issue_date, 'YYYY-MM-DD')::text AS item_date,
           COALESCE(i.customer_name, '')::text AS description,
           i.invoice_number::text AS reference, i.direction::text AS detail
    FROM eng
    JOIN invoices i ON i.taxpayer_id = eng.taxpayer_id
    WHERE @item_type::text IN ('', 'invoice')
      AND i.status_normalized = 'active'
      AND CASE WHEN i.direction = 'incoming' THEN COALESCE(i.vat_deduction_period, i.period) ELSE i.period END = @period::text
      AND engagement_document_visible(eng.id, i.issue_date)
      AND NOT EXISTS (SELECT 1 FROM invoice_journal_links l
                      WHERE l.organization_id = eng.organization_id AND l.engagement_id = eng.id AND l.invoice_id = i.id)
      AND NOT EXISTS (SELECT 1 FROM suggestions s
                      WHERE s.organization_id = eng.organization_id AND s.engagement_id = eng.id AND s.status = 'pending'
                        AND ((s.target_type = 'invoice' AND s.target_id = i.id)
                          OR (s.target_type = 'invoice_item' AND s.target_id IN (
                              SELECT it.id FROM invoice_items it WHERE it.taxpayer_id = i.taxpayer_id AND it.invoice_id = i.id))))
    ORDER BY i.issue_date DESC, i.id DESC
    LIMIT @cap
),
bank AS (
    SELECT 'bank_transaction'::text AS item_type, b.id AS item_id, b.uuid AS item_uuid,
           eng.uuid AS engagement_uuid, eng.title AS taxpayer_title,
           b.amount::numeric(18,2)::text AS amount, b.currency::text AS currency,
           to_char(b.transaction_date, 'YYYY-MM-DD')::text AS item_date,
           COALESCE(NULLIF(b.counterparty_name, ''), b.description)::text AS description,
           COALESCE(b.bank_reference, '')::text AS reference, b.debit_credit::text AS detail
    FROM eng
    JOIN bank_transactions b ON b.taxpayer_id = eng.taxpayer_id
    WHERE @item_type::text IN ('', 'bank_transaction')
      AND b.period = @period::text AND NOT b.is_cancelled
      AND b.match_status NOT IN ('matched', 'ignored')
      AND engagement_document_visible(eng.id, b.transaction_date)
      AND NOT EXISTS (SELECT 1 FROM bank_transaction_postings bp
                      WHERE bp.organization_id = eng.organization_id AND bp.engagement_id = eng.id
                        AND bp.bank_transaction_id = b.id AND bp.status = 'approved')
    ORDER BY b.transaction_date DESC, b.id DESC
    LIMIT @cap
),
exp AS (
    SELECT 'expense'::text AS item_type, x.id AS item_id, x.uuid AS item_uuid,
           eng.uuid AS engagement_uuid, eng.title AS taxpayer_title,
           COALESCE(x.amount::numeric(18,2)::text, '')::text AS amount, x.currency::text AS currency,
           COALESCE(to_char(x.expense_date, 'YYYY-MM-DD'), to_char(x.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM-DD'))::text AS item_date,
           COALESCE(NULLIF(x.vendor_name, ''), x.description)::text AS description,
           COALESCE(x.receipt_number, '')::text AS reference, x.receipt_type::text AS detail
    FROM eng
    JOIN expenses x ON x.taxpayer_id = eng.taxpayer_id
    WHERE @item_type::text IN ('', 'expense')
      AND x.period = @period::text AND x.approval_status = 'pending'
    ORDER BY x.expense_date DESC NULLS LAST, x.id DESC
    LIMIT @cap
),
zr AS (
    SELECT 'retail_sale'::text AS item_type, r.id AS item_id, r.uuid AS item_uuid,
           eng.uuid AS engagement_uuid, eng.title AS taxpayer_title,
           COALESCE(r.total_amount::numeric(18,2)::text, '')::text AS amount, r.currency::text AS currency,
           COALESCE(to_char(r.sale_date, 'YYYY-MM-DD'), to_char(r.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM-DD'))::text AS item_date,
           COALESCE(r.okc_serial_number, '')::text AS description,
           COALESCE(r.z_report_number, '')::text AS reference, ''::text AS detail
    FROM eng
    JOIN retail_sales r ON r.taxpayer_id = eng.taxpayer_id
    WHERE @item_type::text IN ('', 'retail_sale')
      AND r.period = @period::text AND r.approval_status = 'pending'
    ORDER BY r.sale_date DESC NULLS LAST, r.id DESC
    LIMIT @cap
),
fx AS (
    SELECT 'foreign_invoice'::text AS item_type, f.id AS item_id, f.uuid AS item_uuid,
           eng.uuid AS engagement_uuid, eng.title AS taxpayer_title,
           CASE WHEN f.amount > 0 THEN f.amount::numeric(18,2)::text ELSE '' END::text AS amount, f.currency::text AS currency,
           COALESCE(to_char(f.invoice_date, 'YYYY-MM-DD'), to_char(f.created_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM-DD'))::text AS item_date,
           f.vendor::text AS description, f.invoice_no::text AS reference, COALESCE(f.source_kind, '')::text AS detail
    FROM eng
    JOIN foreign_service_invoices f ON f.organization_id = eng.organization_id AND f.engagement_id = eng.id
    WHERE @item_type::text IN ('', 'foreign_invoice')
      AND f.period = @period::text AND f.status = 'draft' AND f.source_kind IS NOT NULL
    ORDER BY f.created_at DESC, f.id DESC
    LIMIT @cap
),
late AS (
    SELECT 'late_document'::text AS item_type, ld.id AS item_id, ld.uuid AS item_uuid,
           eng.uuid AS engagement_uuid, eng.title AS taxpayer_title,
           COALESCE(li.payable_amount, lx.amount, lr.total_amount)::numeric(18,2)::text AS amount,
           COALESCE(li.currency::text, lx.currency::text, lr.currency::text, 'TRY')::text AS currency,
           to_char(ld.arrived_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM-DD')::text AS item_date,
           COALESCE(li.customer_name, lx.vendor_name, NULLIF(lx.description, ''), lr.okc_serial_number, '')::text AS description,
           COALESCE(li.invoice_number::text, lx.receipt_number, lr.z_report_number, '')::text AS reference,
           (ld.document_type || ':' || ld.period)::text AS detail
    FROM eng
    JOIN late_documents ld ON ld.organization_id = eng.organization_id AND ld.engagement_id = eng.id
    LEFT JOIN invoices li ON ld.document_type = 'invoice' AND li.taxpayer_id = eng.taxpayer_id AND li.id = ld.document_id
    LEFT JOIN expenses lx ON ld.document_type = 'expense' AND lx.taxpayer_id = eng.taxpayer_id AND lx.id = ld.document_id
    LEFT JOIN retail_sales lr ON ld.document_type = 'retail_sale' AND lr.taxpayer_id = eng.taxpayer_id AND lr.id = ld.document_id
    WHERE @item_type::text IN ('', 'late_document')
      AND ld.status = 'pending'
    ORDER BY ld.arrived_at DESC, ld.id DESC
    LIMIT @cap
),
items AS (
    SELECT * FROM inv
    UNION ALL SELECT * FROM bank
    UNION ALL SELECT * FROM exp
    UNION ALL SELECT * FROM zr
    UNION ALL SELECT * FROM fx
    UNION ALL SELECT * FROM late
)
SELECT items.item_type::text AS item_type, items.item_id::bigint AS item_id, items.item_uuid::uuid AS item_uuid,
       items.engagement_uuid::uuid AS engagement_uuid, items.taxpayer_title::text AS taxpayer_title,
       COALESCE(items.amount, '')::text AS amount, items.currency::text AS currency, items.item_date::text AS item_date,
       items.description::text AS description, items.reference::text AS reference, items.detail::text AS detail
FROM items
ORDER BY items.item_date DESC, items.item_type, items.item_id DESC
LIMIT @lim OFFSET @off;

-- tenant-lint: org-wide ofisin gelen kutusu sayaçları; read_all yoksa yalnız atanan işler (engagement_assignments)
-- name: CountInboxItems :one
WITH eng AS (
    SELECT e.id, e.organization_id, e.taxpayer_id
    FROM engagements e
    WHERE e.organization_id = @organization_id AND e.ended_at IS NULL
      AND (sqlc.narg(engagement_uuid)::uuid IS NULL OR e.uuid = sqlc.narg(engagement_uuid)::uuid)
      AND (@all_engagements::bool OR EXISTS (
          SELECT 1 FROM engagement_assignments a
          WHERE a.organization_id = e.organization_id AND a.engagement_id = e.id AND a.user_id = @user_id))
)
SELECT
    (SELECT count(*) FROM eng JOIN invoices i ON i.taxpayer_id = eng.taxpayer_id
     WHERE i.status_normalized = 'active'
       AND CASE WHEN i.direction = 'incoming' THEN COALESCE(i.vat_deduction_period, i.period) ELSE i.period END = @period::text
       AND engagement_document_visible(eng.id, i.issue_date)
       AND NOT EXISTS (SELECT 1 FROM invoice_journal_links l
                       WHERE l.organization_id = eng.organization_id AND l.engagement_id = eng.id AND l.invoice_id = i.id)
       AND NOT EXISTS (SELECT 1 FROM suggestions s
                       WHERE s.organization_id = eng.organization_id AND s.engagement_id = eng.id AND s.status = 'pending'
                         AND ((s.target_type = 'invoice' AND s.target_id = i.id)
                           OR (s.target_type = 'invoice_item' AND s.target_id IN (
                               SELECT it.id FROM invoice_items it WHERE it.taxpayer_id = i.taxpayer_id AND it.invoice_id = i.id)))))::bigint AS invoice,
    (SELECT count(*) FROM eng JOIN bank_transactions b ON b.taxpayer_id = eng.taxpayer_id
     WHERE b.period = @period::text AND NOT b.is_cancelled
       AND b.match_status NOT IN ('matched', 'ignored')
       AND engagement_document_visible(eng.id, b.transaction_date)
       AND NOT EXISTS (SELECT 1 FROM bank_transaction_postings bp
                       WHERE bp.organization_id = eng.organization_id AND bp.engagement_id = eng.id
                         AND bp.bank_transaction_id = b.id AND bp.status = 'approved'))::bigint AS bank_transaction,
    (SELECT count(*) FROM eng JOIN expenses x ON x.taxpayer_id = eng.taxpayer_id
     WHERE x.period = @period::text AND x.approval_status = 'pending')::bigint AS expense,
    (SELECT count(*) FROM eng JOIN retail_sales r ON r.taxpayer_id = eng.taxpayer_id
     WHERE r.period = @period::text AND r.approval_status = 'pending')::bigint AS retail_sale,
    (SELECT count(*) FROM eng JOIN foreign_service_invoices f ON f.organization_id = eng.organization_id AND f.engagement_id = eng.id
     WHERE f.period = @period::text AND f.status = 'draft' AND f.source_kind IS NOT NULL)::bigint AS foreign_invoice,
    (SELECT count(*) FROM eng JOIN late_documents ld ON ld.organization_id = eng.organization_id AND ld.engagement_id = eng.id
     WHERE ld.status = 'pending')::bigint AS late_document;
